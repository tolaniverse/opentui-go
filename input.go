package opentui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// InputOptions configures a single-line editable text node.
type InputOptions struct {
	ID     string
	Style  Style
	ZIndex int

	Value       string
	Placeholder string
	MinLength   int // grapheme clusters
	MaxLength   int // grapheme clusters; zero uses 1000

	FG, BG               RGBA
	FocusedFG, FocusedBG RGBA
	PlaceholderColor     RGBA

	OnInput  func(value string) // every edit
	OnChange func(value string) // blur or submit after a change
	OnSubmit func(value string)
}

type inputState struct {
	value       []rune
	cursor      int
	viewStart   int
	placeholder string
	minLength   int
	maxLength   int
	committed   string
	history     inputHistory
	opts        InputOptions
}

// Input returns a focusable, measured, single-line editor. Use Renderer.Focus
// to focus it initially; Tab and Shift+Tab traverse all focusable nodes.
func Input(o InputOptions) *InputNode {
	if o.MaxLength == 0 {
		o.MaxLength = 1000
	}
	if o.MinLength < 0 || o.MaxLength < 0 || o.MinLength > o.MaxLength {
		panic("opentui: invalid Input MinLength/MaxLength")
	}
	if o.Style.Height.IsAuto() {
		o.Style.Height = Cells(1)
	}
	if o.FG == (RGBA{}) {
		o.FG = White
	}
	if o.FocusedFG == (RGBA{}) {
		o.FocusedFG = o.FG
	}
	if o.FocusedBG == (RGBA{}) {
		o.FocusedBG = o.BG
	}
	if o.PlaceholderColor == (RGBA{}) {
		o.PlaceholderColor = RGB(128, 128, 128)
	}

	n := NewNode(o.Style)
	n.ID, n.ZIndex, n.Focusable = o.ID, o.ZIndex, true
	n.input = &inputState{placeholder: o.Placeholder, minLength: o.MinLength, maxLength: o.MaxLength, opts: o}
	n.setInputValue(o.Value, false)
	n.input.committed = n.value()
	n.input.cursor = len(n.input.value)

	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		content := max(displayWidth(n.value()), displayWidth(n.input.placeholder))
		return fit(float64(content), w, wm), fit(1, h, hm)
	}
	n.paint = paintInput
	n.keyHandler = n.handleInputKey
	n.pasteHandler = func(event *PasteEvent) {
		if n.insertText(event.Text) {
			event.PreventDefault()
		}
	}
	n.onFocus = func() { n.input.committed = n.value() }
	n.onBlur = func() { n.commitInput() }
	return &InputNode{n}
}

// value returns an Input or Textarea node's text (Textarea lines are joined
// with "\n").
func (n *Node) value() string {
	if n.textarea != nil {
		return n.textarea.value()
	}
	if n.input == nil {
		return ""
	}
	return string(n.input.value)
}

// SetValue replaces an Input node's text, strips newlines, enforces MaxLength
// in whole grapheme clusters,
// moves the cursor to the end, and invokes OnInput when the value changes.
// On a Textarea it keeps newlines, normalizes line endings, and expands tabs.
func (n *Node) setValue(value string) {
	if n.textarea != nil {
		n.setTextareaValue(value, true)
		return
	}
	n.setInputValue(value, true)
}

func (n *Node) setInputValue(value string, notify bool) {
	if n.input == nil {
		return
	}
	value = strings.NewReplacer("\r", "", "\n", "").Replace(value)
	runes := []rune(truncateClusters(value, n.input.maxLength))
	if string(runes) == string(n.input.value) {
		return
	}
	n.input.value = runes
	n.input.cursor = len(runes)
	n.input.history = inputHistory{} // as upstream setText: a new value starts a new history
	if notify && n.input.opts.OnInput != nil {
		n.input.opts.OnInput(string(runes))
	}
}

// Cursor returns an Input node's cursor offset in runes, always at a grapheme
// boundary. Rune offsets are retained for API compatibility with v0.3–0.6.
func (n *Node) cursor() int {
	if n.input == nil {
		return 0
	}
	return n.input.cursor
}

// SetCursor moves an Input cursor, clamped to [0, len(value)].
func (n *Node) setCursor(offset int) {
	if n.input != nil {
		n.input.cursor = inputFloor(n.input.value, offset)
	}
}

// InsertText inserts sanitized text at the cursor. It reports whether the
// value changed.
func (n *Node) insertText(text string) bool {
	if n.textarea != nil {
		return n.insertTextarea(text, editOther)
	}
	if n.input == nil {
		return false
	}
	return n.insertInput(text, editOther)
}

// insertInput inserts sanitized text at the cursor as one undo step of
// kind (typed keys merge into words).
func (n *Node) insertInput(text string, kind editKind) bool {
	text = strings.NewReplacer("\r", "", "\n", "").Replace(text)
	if text == "" {
		return false
	}
	i := n.input.cursor
	insert := []rune(text)
	whole := make([]rune, 0, len(n.input.value)+len(insert))
	whole = append(whole, n.input.value[:i]...)
	whole = append(whole, insert...)
	whole = append(whole, n.input.value[i:]...)
	if len(inputOffsets(whole))-1 > n.input.maxLength {
		// The whole paste does not fit. Fall back to prefix checks so an
		// insertion that joins either neighboring cluster still counts right.
		insert = insert[:0]
		for g := uniseg.NewGraphemes(text); g.Next(); {
			trial := append(append(append([]rune{}, n.input.value[:i]...), insert...), []rune(g.Str())...)
			trial = append(trial, n.input.value[i:]...)
			if len(inputOffsets(trial))-1 > n.input.maxLength {
				break
			}
			insert = append(insert, []rune(g.Str())...)
		}
	}
	if len(insert) == 0 {
		return false
	}
	n.inputCheckpoint(kind, string(insert))
	value := make([]rune, 0, len(n.input.value)+len(insert))
	value = append(value, n.input.value[:i]...)
	value = append(value, insert...)
	value = append(value, n.input.value[i:]...)
	n.input.value = value
	n.input.cursor = inputCeil(value, i+len(insert))
	n.inputEdited()
	n.notifyInput()
	return true
}

// Submit invokes OnChange if needed and then OnSubmit when MinLength is met.
func (n *Node) submit() bool {
	if n.textarea != nil {
		return n.submitTextarea()
	}
	if n.input == nil || len(inputOffsets(n.input.value))-1 < n.input.minLength {
		return false
	}
	n.commitInput()
	if n.input.opts.OnSubmit != nil {
		n.input.opts.OnSubmit(n.value())
	}
	return true
}

func (n *Node) notifyInput() {
	if n.input.opts.OnInput != nil {
		n.input.opts.OnInput(n.value())
	}
}

func (n *Node) commitInput() {
	if n.input == nil {
		return
	}
	value := n.value()
	if value != n.input.committed {
		n.input.committed = value
		if n.input.opts.OnChange != nil {
			n.input.opts.OnChange(value)
		}
	}
}

func (n *Node) handleInputKey(key *KeyEvent) {
	if n.input == nil || key.Type == KeyRelease {
		return
	}
	s := n.input
	handled := true
	// cut deletes [a, b) as one undo step; an empty range does nothing.
	cut := func(a, b int, kind editKind) {
		if a >= b {
			handled = false
			return
		}
		n.inputCheckpoint(kind, "")
		s.value = append(s.value[:a], s.value[b:]...)
		s.cursor = inputFloor(s.value, a)
		n.inputEdited()
		n.notifyInput()
	}
	move := func(to int) {
		if to == s.cursor {
			handled = false
		}
		s.cursor = to
	}
	switch {
	case isUndoKey(key):
		n.undo()
	case isRedoKey(key):
		n.redo()
	case isWordLeftKey(key):
		move(prevWordBoundary(s.value, s.cursor))
	case isWordRightKey(key):
		move(nextWordBoundary(s.value, s.cursor))
	case isDeleteWordBackKey(key):
		cut(prevWordBoundary(s.value, s.cursor), s.cursor, editOther)
	case isDeleteWordForwardKey(key):
		cut(s.cursor, nextWordBoundary(s.value, s.cursor), editOther)
	case key.Ctrl && key.Name == "a":
		s.cursor = 0
	case key.Ctrl && key.Name == "e":
		s.cursor = len(s.value)
	case key.Ctrl && key.Name == "u":
		cut(0, s.cursor, editOther)
	case key.Ctrl && key.Name == "k":
		cut(s.cursor, len(s.value), editOther)
	case key.Name == "left" || (key.Ctrl && key.Name == "b"):
		move(max(inputPrev(s.value, s.cursor), 0))
	case key.Name == "right" || (key.Ctrl && key.Name == "f"):
		move(min(inputNext(s.value, s.cursor), len(s.value)))
	case key.Name == "home":
		s.cursor = 0
	case key.Name == "end":
		s.cursor = len(s.value)
	case key.Name == "backspace":
		if s.cursor == 0 {
			handled = false
			break
		}
		cut(inputPrev(s.value, s.cursor), s.cursor, editBack)
	case key.Name == "delete" || (key.Ctrl && key.Name == "d"):
		if s.cursor == len(s.value) {
			handled = false
			break
		}
		cut(s.cursor, inputNext(s.value, s.cursor), editDelete)
	case key.Name == "return" || key.Name == "kpenter" || key.Name == "linefeed":
		handled = n.submit()
	case !key.Ctrl && !key.Alt && !key.Meta && !key.Super && printableSequence(key.Sequence):
		handled = n.insertInput(key.Sequence, editType)
	default:
		handled = false
	}
	if handled {
		key.PreventDefault()
	}
}

func printableSequence(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '\r' || r == '\n' || !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func paintInput(b *Buffer, n *Node) {
	s := n.input
	r := n.rect.Inset(n.Style.Padding)
	if r.Empty() {
		return
	}
	fg, bg := s.opts.FG, s.opts.BG
	if n.focused {
		fg, bg = s.opts.FocusedFG, s.opts.FocusedBG
	}
	if bg.A > 0 {
		b.FillRect(r.X, r.Y, r.W, r.H, bg)
	}

	if len(s.value) == 0 && !n.focused {
		b.DrawText(s.placeholder, r.X, r.Y, s.opts.PlaceholderColor, bg, AttrDim)
		return
	}
	// Segment once for this paint. Repeated width/slice calculations in the
	// scrolling loop are quadratic for long Unicode values.
	type part struct{ start, end, width int }
	parts := make([]part, 0, len(s.value))
	runePos := 0
	for g := uniseg.NewGraphemes(string(s.value)); g.Next(); {
		end := runePos + utf8.RuneCountInString(g.Str())
		parts = append(parts, part{runePos, end, clusterWidth(g.Str(), g.Width())})
		runePos = end
	}
	startIndex, cursorIndex := 0, len(parts)
	for i, p := range parts {
		if p.start < s.viewStart {
			startIndex = i + 1
		}
		if p.start == s.cursor {
			cursorIndex = i
		}
	}
	if s.cursor < s.viewStart {
		startIndex = cursorIndex
	}
	column := 0
	for _, p := range parts[startIndex:cursorIndex] {
		column += p.width
	}
	glyphWidth := 1
	if cursorIndex < len(parts) {
		glyphWidth = parts[cursorIndex].width
	}
	for startIndex < cursorIndex && (column >= r.W || column+glyphWidth > r.W) {
		column -= parts[startIndex].width
		startIndex++
	}
	s.viewStart = len(s.value)
	if startIndex < len(parts) {
		s.viewStart = parts[startIndex].start
	}
	end, used := s.viewStart, 0
	for _, p := range parts[startIndex:] {
		if used+p.width > r.W {
			break
		}
		used += p.width
		end = p.end
	}
	if end > s.viewStart {
		b.DrawText(string(s.value[s.viewStart:end]), r.X, r.Y, fg, bg, AttrNone)
	}
	if n.focused {
		if x := r.X + column; x >= r.X && x < r.X+r.W {
			b.drawCursor(x, r.Y)
		}
	}
}

// inputOffsets maps grapheme boundaries to rune offsets. Cursor's public
// unit remains runes, while movement/deletion always use these boundaries.
func inputOffsets(value []rune) []int {
	offsets := []int{0}
	pos := 0
	for g := uniseg.NewGraphemes(string(value)); g.Next(); {
		pos += utf8.RuneCountInString(g.Str())
		offsets = append(offsets, pos)
	}
	return offsets
}

func inputFloor(value []rune, offset int) int {
	result := 0
	for _, boundary := range inputOffsets(value) {
		if boundary > offset {
			break
		}
		result = boundary
	}
	return result
}

func inputCeil(value []rune, offset int) int {
	for _, boundary := range inputOffsets(value) {
		if boundary >= offset {
			return boundary
		}
	}
	return len(value)
}

func inputPrev(value []rune, offset int) int { return inputFloor(value, offset-1) }
func inputNext(value []rune, offset int) int { return inputCeil(value, offset+1) }

func truncateClusters(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	g := uniseg.NewGraphemes(value)
	count, end := 0, 0
	for g.Next() && count < limit {
		_, end = g.Positions()
		count++
	}
	return value[:end]
}
