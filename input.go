package opentui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// InputOptions configures a single-line editable text node.
type InputOptions struct {
	ID     string
	Style  Style
	ZIndex int

	Value       string
	Placeholder string
	MinLength   int
	MaxLength   int // zero uses 1000

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
	opts        InputOptions
}

// Input returns a focusable, measured, single-line editor. Use Renderer.Focus
// to focus it initially; Tab and Shift+Tab traverse all focusable nodes.
func Input(o InputOptions) *Node {
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
	n.input.committed = n.Value()
	n.input.cursor = len(n.input.value)

	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		content := max(len(n.input.value), utf8.RuneCountInString(n.input.placeholder))
		return fit(float64(content), w, wm), fit(1, h, hm)
	}
	n.paint = paintInput
	n.keyHandler = n.handleInputKey
	n.pasteHandler = func(event *PasteEvent) {
		if n.InsertText(event.Text) {
			event.PreventDefault()
		}
	}
	n.onFocus = func() { n.input.committed = n.Value() }
	n.onBlur = func() { n.commitInput() }
	return n
}

// Value returns an Input node's text, or "" for other node types.
func (n *Node) Value() string {
	if n.input == nil {
		return ""
	}
	return string(n.input.value)
}

// SetValue replaces an Input node's text, strips newlines, enforces MaxLength,
// moves the cursor to the end, and invokes OnInput when the value changes.
func (n *Node) SetValue(value string) { n.setInputValue(value, true) }

func (n *Node) setInputValue(value string, notify bool) {
	if n.input == nil {
		return
	}
	value = strings.NewReplacer("\r", "", "\n", "").Replace(value)
	runes := []rune(value)
	if len(runes) > n.input.maxLength {
		runes = runes[:n.input.maxLength]
	}
	if string(runes) == string(n.input.value) {
		return
	}
	n.input.value = runes
	n.input.cursor = len(runes)
	if notify && n.input.opts.OnInput != nil {
		n.input.opts.OnInput(string(runes))
	}
}

// Cursor returns an Input node's cursor offset in runes.
func (n *Node) Cursor() int {
	if n.input == nil {
		return 0
	}
	return n.input.cursor
}

// SetCursor moves an Input cursor, clamped to [0, len(value)].
func (n *Node) SetCursor(offset int) {
	if n.input != nil {
		n.input.cursor = min(max(offset, 0), len(n.input.value))
	}
}

// InsertText inserts sanitized text at the cursor. It reports whether the
// value changed.
func (n *Node) InsertText(text string) bool {
	if n.input == nil {
		return false
	}
	text = strings.NewReplacer("\r", "", "\n", "").Replace(text)
	insert := []rune(text)
	remaining := n.input.maxLength - len(n.input.value)
	if remaining <= 0 || len(insert) == 0 {
		return false
	}
	if len(insert) > remaining {
		insert = insert[:remaining]
	}
	i := n.input.cursor
	value := make([]rune, 0, len(n.input.value)+len(insert))
	value = append(value, n.input.value[:i]...)
	value = append(value, insert...)
	value = append(value, n.input.value[i:]...)
	n.input.value = value
	n.input.cursor += len(insert)
	n.notifyInput()
	return true
}

// Submit invokes OnChange if needed and then OnSubmit when MinLength is met.
func (n *Node) Submit() bool {
	if n.input == nil || len(n.input.value) < n.input.minLength {
		return false
	}
	n.commitInput()
	if n.input.opts.OnSubmit != nil {
		n.input.opts.OnSubmit(n.Value())
	}
	return true
}

func (n *Node) notifyInput() {
	if n.input.opts.OnInput != nil {
		n.input.opts.OnInput(n.Value())
	}
}

func (n *Node) commitInput() {
	if n.input == nil {
		return
	}
	value := n.Value()
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
	switch {
	case key.Ctrl && key.Name == "a":
		s.cursor = 0
	case key.Ctrl && key.Name == "e":
		s.cursor = len(s.value)
	case key.Ctrl && key.Name == "u":
		if s.cursor == 0 {
			handled = false
			break
		}
		s.value = append([]rune(nil), s.value[s.cursor:]...)
		s.cursor = 0
		n.notifyInput()
	case key.Ctrl && key.Name == "k":
		if s.cursor == len(s.value) {
			handled = false
			break
		}
		s.value = append([]rune(nil), s.value[:s.cursor]...)
		n.notifyInput()
	case key.Ctrl && key.Name == "w":
		start := s.cursor
		for start > 0 && unicode.IsSpace(s.value[start-1]) {
			start--
		}
		for start > 0 && !unicode.IsSpace(s.value[start-1]) {
			start--
		}
		if start == s.cursor {
			handled = false
			break
		}
		s.value = append(s.value[:start], s.value[s.cursor:]...)
		s.cursor = start
		n.notifyInput()
	case key.Name == "left":
		if s.cursor > 0 {
			s.cursor--
		} else {
			handled = false
		}
	case key.Name == "right":
		if s.cursor < len(s.value) {
			s.cursor++
		} else {
			handled = false
		}
	case key.Name == "home":
		s.cursor = 0
	case key.Name == "end":
		s.cursor = len(s.value)
	case key.Name == "backspace":
		if s.cursor == 0 {
			handled = false
			break
		}
		s.value = append(s.value[:s.cursor-1], s.value[s.cursor:]...)
		s.cursor--
		n.notifyInput()
	case key.Name == "delete":
		if s.cursor == len(s.value) {
			handled = false
			break
		}
		s.value = append(s.value[:s.cursor], s.value[s.cursor+1:]...)
		n.notifyInput()
	case key.Name == "return" || key.Name == "kpenter" || key.Name == "linefeed":
		handled = n.Submit()
	case !key.Ctrl && !key.Alt && printableSequence(key.Sequence):
		handled = n.InsertText(key.Sequence)
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
	if s.cursor < s.viewStart {
		s.viewStart = s.cursor
	}
	if s.cursor >= s.viewStart+r.W {
		s.viewStart = s.cursor - r.W + 1
	}
	s.viewStart = min(s.viewStart, max(len(s.value)-r.W+1, 0))
	end := min(s.viewStart+r.W, len(s.value))
	if end > s.viewStart {
		b.DrawText(string(s.value[s.viewStart:end]), r.X, r.Y, fg, bg, AttrNone)
	}
	if n.focused {
		x := r.X + s.cursor - s.viewStart
		if x >= r.X && x < r.X+r.W {
			char := ' '
			if s.cursor < len(s.value) {
				char = s.value[s.cursor]
			}
			b.SetBlended(x, r.Y, Cell{Char: char, FG: fg, BG: bg, Attr: AttrInverse})
		}
	}
}
