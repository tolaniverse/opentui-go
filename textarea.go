package opentui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// TextareaOptions configures a multiline editable text node.
//
// Key bindings follow a subset of upstream's defaults (Textarea.ts):
// Enter inserts a newline and Meta/Alt+Enter submits; Home/End jump to the
// start/end of the buffer and Ctrl+A/E to the start/end of the line.
type TextareaOptions struct {
	ID     string
	Style  Style
	ZIndex int

	Value       string
	Placeholder string

	FG, BG               RGBA
	FocusedFG, FocusedBG RGBA
	PlaceholderColor     RGBA

	OnInput  func(value string) // every edit
	OnChange func(value string) // blur or submit after a change
	OnSubmit func(value string) // Meta/Alt+Enter
}

// tabWidth is how many spaces a pasted or typed tab becomes. Cells hold
// no tab stops, so tabs are expanded on input.
const tabWidth = 4

type textareaState struct {
	lines     [][]rune // never empty; no line contains '\n'
	line, col int      // cursor; col is a rune offset at a grapheme boundary
	// goal is the terminal-cell column Up/Down aim for, so moving through
	// a short line does not lose the column. -1 means "use the cursor's".
	goal             int
	scrollX, scrollY int // first visible cell column and line
	committed        string
	opts             TextareaOptions
}

// Textarea returns a focusable, measured, multiline editor. Lines do not
// soft-wrap; the view scrolls horizontally and vertically to the cursor.
func Textarea(o TextareaOptions) *Node {
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
	n.textarea = &textareaState{lines: [][]rune{nil}, goal: -1, opts: o}
	n.setTextareaValue(o.Value, false)
	n.textarea.committed = n.Value()

	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		s := n.textarea
		width := 0
		for _, line := range s.lines {
			width = max(width, displayWidth(string(line)))
		}
		// One extra column so a cursor at the end of the longest line fits.
		width = max(width+1, displayWidth(s.opts.Placeholder))
		return fit(float64(width), w, wm), fit(float64(len(s.lines)), h, hm)
	}
	n.paint = paintTextarea
	n.keyHandler = n.handleTextareaKey
	n.pasteHandler = func(event *PasteEvent) {
		if n.InsertText(event.Text) {
			event.PreventDefault()
		}
	}
	n.onFocus = func() { n.textarea.committed = n.Value() }
	n.onBlur = func() { n.commitTextarea() }
	return n
}

// sanitizeTextarea normalizes line endings to "\n", expands tabs, and drops
// other control characters, which would corrupt the screen.
func sanitizeTextarea(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\r' || r == '\n':
			b.WriteByte('\n')
		case r == '\t':
			b.WriteString(strings.Repeat(" ", tabWidth))
		case r < 0x20 || (r >= 0x7f && r <= 0x9f):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *textareaState) value() string {
	parts := make([]string, len(s.lines))
	for i, line := range s.lines {
		parts[i] = string(line)
	}
	return strings.Join(parts, "\n")
}

func (n *Node) setTextareaValue(value string, notify bool) {
	s := n.textarea
	value = sanitizeTextarea(value)
	if value == s.value() {
		return
	}
	split := strings.Split(value, "\n")
	s.lines = make([][]rune, len(split))
	for i, line := range split {
		s.lines[i] = []rune(line)
	}
	s.line = len(s.lines) - 1
	s.col = len(s.lines[s.line])
	s.goal = -1
	if notify {
		n.notifyTextarea()
	}
}

// CursorPosition returns a Textarea's cursor as a line index and a rune
// offset within that line, always at a grapheme boundary. Other node types
// return 0, 0.
func (n *Node) CursorPosition() (line, col int) {
	if n.textarea == nil {
		return 0, 0
	}
	return n.textarea.line, n.textarea.col
}

// SetCursorPosition moves a Textarea cursor. The line is clamped, and the
// rune offset is clamped and snapped to the preceding grapheme boundary.
func (n *Node) SetCursorPosition(line, col int) {
	if n.textarea == nil {
		return
	}
	s := n.textarea
	s.line = min(max(line, 0), len(s.lines)-1)
	s.col = inputFloor(s.lines[s.line], col)
	s.goal = -1
}

// LineCount returns the number of lines in a Textarea (at least 1), or 0
// for other node types.
func (n *Node) LineCount() int {
	if n.textarea == nil {
		return 0
	}
	return len(n.textarea.lines)
}

func (n *Node) insertTextarea(text string) bool {
	s := n.textarea
	text = sanitizeTextarea(text)
	if text == "" {
		return false
	}
	cur := s.lines[s.line]
	if !strings.Contains(text, "\n") {
		// Typing: edit one line instead of rebuilding the line list.
		insert := []rune(text)
		line := make([]rune, 0, len(cur)+len(insert))
		line = append(append(append(line, cur[:s.col]...), insert...), cur[s.col:]...)
		s.lines[s.line] = line
		s.col = inputCeil(line, s.col+len(insert))
		s.goal = -1
		n.notifyTextarea()
		return true
	}
	parts := strings.Split(text, "\n")
	before := append([]rune(nil), cur[:s.col]...)
	after := append([]rune(nil), cur[s.col:]...)

	added := make([][]rune, len(parts))
	for i, p := range parts {
		added[i] = []rune(p)
	}
	added[0] = append(before, added[0]...)
	last := len(added) - 1
	col := len(added[last])
	added[last] = append(added[last], after...)

	lines := make([][]rune, 0, len(s.lines)+last)
	lines = append(lines, s.lines[:s.line]...)
	lines = append(lines, added...)
	lines = append(lines, s.lines[s.line+1:]...)
	s.lines = lines
	s.line += last
	// Inserted combining marks can merge with the following text, so the
	// end of the insertion may be inside a cluster: move past it.
	s.col = inputCeil(s.lines[s.line], col)
	s.goal = -1
	n.notifyTextarea()
	return true
}

func (n *Node) notifyTextarea() {
	if fn := n.textarea.opts.OnInput; fn != nil {
		fn(n.Value())
	}
}

func (n *Node) commitTextarea() {
	s := n.textarea
	if v := n.Value(); v != s.committed {
		s.committed = v
		if s.opts.OnChange != nil {
			s.opts.OnChange(v)
		}
	}
}

func (n *Node) submitTextarea() bool {
	n.commitTextarea()
	if fn := n.textarea.opts.OnSubmit; fn != nil {
		fn(n.Value())
	}
	return true
}

// joinWithNext merges line i+1 into line i, keeping the cursor at the join.
func (s *textareaState) joinWithNext(i int) {
	s.line, s.col = i, len(s.lines[i])
	s.lines[i] = append(s.lines[i], s.lines[i+1]...)
	s.lines = append(s.lines[:i+1], s.lines[i+2:]...)
	// The join can create a cluster across the old boundary (a leading
	// combining mark), so keep the cursor on a boundary.
	s.col = inputFloor(s.lines[i], s.col)
}

// cellColumn is the terminal-cell column of a rune offset in a line.
func cellColumn(line []rune, col int) int { return displayWidth(string(line[:col])) }

// colAtCell returns the last grapheme boundary in line whose cell column
// is at most cell.
func colAtCell(line []rune, cell int) int {
	if asciiOnlyRunes(line) {
		return min(max(cell, 0), len(line))
	}
	col, used := 0, 0
	for g := uniseg.NewGraphemes(string(line)); g.Next(); {
		used += max(g.Width(), 1)
		if used > cell {
			break
		}
		col += utf8.RuneCountInString(g.Str())
	}
	return col
}

func asciiOnlyRunes(line []rune) bool {
	for _, r := range line {
		if r >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func (n *Node) handleTextareaKey(key *KeyEvent) {
	if n.textarea == nil || key.Type == KeyRelease {
		return
	}
	s := n.textarea
	line := s.lines[s.line]
	edited, handled := false, true
	vertical := false
	meta := key.Alt || key.Meta

	switch {
	case (key.Name == "return" || key.Name == "kpenter") && meta:
		handled = n.submitTextarea()
	case key.Name == "return" || key.Name == "kpenter" || key.Name == "linefeed":
		handled = n.insertTextarea("\n")
	case key.Name == "up", key.Name == "down":
		vertical = true
		if s.goal < 0 {
			s.goal = cellColumn(line, s.col)
		}
		switch {
		case key.Name == "up" && s.line > 0:
			s.line--
			s.col = colAtCell(s.lines[s.line], s.goal)
		case key.Name == "up":
			s.col = 0
		case s.line < len(s.lines)-1:
			s.line++
			s.col = colAtCell(s.lines[s.line], s.goal)
		default:
			s.col = len(line)
		}
	case key.Name == "left" || (key.Ctrl && key.Name == "b"):
		switch {
		case s.col > 0:
			s.col = inputPrev(line, s.col)
		case s.line > 0:
			s.line--
			s.col = len(s.lines[s.line])
		default:
			handled = false
		}
	case key.Name == "right" || (key.Ctrl && key.Name == "f"):
		switch {
		case s.col < len(line):
			s.col = inputNext(line, s.col)
		case s.line < len(s.lines)-1:
			s.line++
			s.col = 0
		default:
			handled = false
		}
	case key.Name == "home":
		s.line, s.col = 0, 0
	case key.Name == "end":
		s.line = len(s.lines) - 1
		s.col = len(s.lines[s.line])
	case key.Ctrl && key.Name == "a":
		s.col = 0
	case key.Ctrl && key.Name == "e":
		s.col = len(line)
	case key.Name == "backspace":
		switch {
		case s.col > 0:
			start := inputPrev(line, s.col)
			s.lines[s.line] = append(line[:start], line[s.col:]...)
			s.col = start
			edited = true
		case s.line > 0:
			s.joinWithNext(s.line - 1)
			edited = true
		default:
			handled = false
		}
	case key.Name == "delete" || (key.Ctrl && key.Name == "d"):
		switch {
		case s.col < len(line):
			s.lines[s.line] = append(line[:s.col], line[inputNext(line, s.col):]...)
			edited = true
		case s.line < len(s.lines)-1:
			s.joinWithNext(s.line)
			edited = true
		default:
			handled = false
		}
	case key.Ctrl && key.Name == "k":
		switch {
		case s.col < len(line):
			s.lines[s.line] = append([]rune(nil), line[:s.col]...)
			edited = true
		case s.line < len(s.lines)-1:
			s.joinWithNext(s.line)
			edited = true
		default:
			handled = false
		}
	case key.Ctrl && key.Name == "u":
		if s.col == 0 {
			handled = false
			break
		}
		s.lines[s.line] = append([]rune(nil), line[s.col:]...)
		s.col = 0
		edited = true
	case key.Ctrl && key.Name == "w":
		if s.col == 0 {
			if s.line == 0 {
				handled = false
				break
			}
			s.joinWithNext(s.line - 1)
			edited = true
			break
		}
		start := s.col
		for start > 0 && unicode.IsSpace(line[inputPrev(line, start)]) {
			start = inputPrev(line, start)
		}
		for start > 0 && !unicode.IsSpace(line[inputPrev(line, start)]) {
			start = inputPrev(line, start)
		}
		s.lines[s.line] = append(line[:start], line[s.col:]...)
		s.col = start
		edited = true
	case !key.Ctrl && !meta && printableSequence(key.Sequence):
		handled = n.insertTextarea(key.Sequence)
	default:
		handled = false
	}
	if !vertical {
		s.goal = -1
	}
	if edited {
		n.notifyTextarea()
	}
	if handled {
		key.PreventDefault()
	}
}

func paintTextarea(b *Buffer, n *Node) {
	s := n.textarea
	r := n.rect.Inset(n.Style.Padding)
	if r.Empty() {
		return
	}
	fg, bg := s.opts.FG, s.opts.BG
	if n.focused {
		fg, bg = s.opts.FocusedFG, s.opts.FocusedBG
	}
	b.PushClip(r)
	defer b.PopClip()
	if bg.A > 0 {
		b.FillRect(r.X, r.Y, r.W, r.H, bg)
	}
	if len(s.lines) == 1 && len(s.lines[0]) == 0 && !n.focused {
		b.DrawText(s.opts.Placeholder, r.X, r.Y, s.opts.PlaceholderColor, bg, AttrDim)
		return
	}

	// Keep the cursor's line and cell inside the viewport.
	s.scrollY = min(s.scrollY, max(len(s.lines)-r.H, 0))
	if s.line < s.scrollY {
		s.scrollY = s.line
	} else if s.line >= s.scrollY+r.H {
		s.scrollY = s.line - r.H + 1
	}
	cur := s.lines[s.line]
	cursorCell := cellColumn(cur, s.col)
	glyph, glyphWidth := " ", 1
	if s.col < len(cur) {
		glyph = string(cur[s.col:inputNext(cur, s.col)])
		glyphWidth = max(displayWidth(glyph), 1)
	}
	if cursorCell < s.scrollX {
		s.scrollX = cursorCell
	} else if cursorCell+glyphWidth > s.scrollX+r.W {
		s.scrollX = cursorCell + glyphWidth - r.W
	}
	s.scrollX = max(s.scrollX, 0)

	for row := 0; row < r.H && s.scrollY+row < len(s.lines); row++ {
		drawTextareaLine(b, s.lines[s.scrollY+row], s.scrollX, r.W, r.X, r.Y+row, fg, bg)
	}
	if n.focused {
		x := r.X + cursorCell - s.scrollX
		if glyphWidth > r.X+r.W-x {
			glyph = " "
		}
		b.DrawText(glyph, x, r.Y+s.line-s.scrollY, fg, bg, AttrInverse)
	}
}

// drawTextareaLine paints the cells [from, from+width) of one line at x, y.
// A cluster cut by either edge is left blank rather than drawn in part.
func drawTextareaLine(b *Buffer, line []rune, from, width, x, y int, fg, bg RGBA) {
	if asciiOnlyRunes(line) {
		if from < len(line) {
			b.DrawText(string(line[from:min(from+width, len(line))]), x, y, fg, bg, AttrNone)
		}
		return
	}
	cell := 0
	for g := uniseg.NewGraphemes(string(line)); g.Next() && cell < from+width; {
		w := max(g.Width(), 1)
		if cell >= from && cell+w <= from+width {
			b.DrawText(g.Str(), x+cell-from, y, fg, bg, AttrNone)
		}
		cell += w
	}
}
