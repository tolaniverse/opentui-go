package opentui

import (
	"strings"
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
	// OnSelect is called with the selected text when a mouse selection
	// ends (on release) with a non-empty selection, e.g. to copy it.
	OnSelect func(text string)

	SelectionFG, SelectionBG RGBA // zero values match Text's defaults

	// Wrap soft-wraps long lines; WrapNone (the zero value) scrolls them
	// horizontally instead. Wrapping keeps the last column free for the
	// cursor. Up/Down then move by visual row.
	Wrap WrapMode
}

// tabWidth is how many spaces a pasted or typed tab becomes. Cells hold
// no tab stops, so tabs are expanded on input.
const tabWidth = 4

type textareaState struct {
	lines     [][]rune // never empty; no line contains '\n'
	line, col int      // cursor; col is a rune offset at a grapheme boundary
	// goal is the terminal-cell column Up/Down aim for, so moving through
	// a short line does not lose the column. -1 means "use the cursor's".
	goal    int
	history textHistory
	scrollX int // first visible cell column (WrapNone only)
	// topLine/topRow is the first visible visual row: a logical line and a
	// row within it (always 0 for WrapNone).
	topLine, topRow int
	// wrapWidth is the width rows were last wrapped to when painting; 0
	// before the first paint or with WrapNone (one row per line).
	wrapWidth int

	selectionState
	// follow keeps the cursor in view when painting. Wheel scrolling turns
	// it off so the view can move away; keys, edits and clicks turn it on.
	follow    bool
	committed string
	opts      TextareaOptions
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
	if o.SelectionFG == (RGBA{}) {
		o.SelectionFG = RGB(30, 30, 46)
	}
	if o.SelectionBG == (RGBA{}) {
		o.SelectionBG = RGB(137, 180, 250)
	}
	n := NewNode(o.Style)
	n.ID, n.ZIndex, n.Focusable = o.ID, o.ZIndex, true
	n.textarea = &textareaState{lines: [][]rune{nil}, goal: -1, follow: true, opts: o}
	n.setTextareaValue(o.Value, false)
	n.textarea.committed = n.Value()

	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		s := n.textarea
		width, height := 0, 0
		wrapAt := 0
		if s.opts.Wrap != WrapNone && wm != MeasureUndefined {
			wrapAt = max(int(w)-1, 1)
		}
		for _, line := range s.lines {
			rows := wrapLine(line, wrapAt, s.opts.Wrap)
			for _, row := range rows {
				width = max(width, row.width)
			}
			height += len(rows)
		}
		// One extra column so a cursor at the end of the longest row fits.
		width = max(width+1, displayWidth(s.opts.Placeholder))
		return fit(float64(width), w, wm), fit(float64(height), h, hm)
	}
	n.paint = paintTextarea
	n.keyHandler = n.handleTextareaKey
	n.controlMouseHandler = n.handleTextareaMouse
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
	s.hasAnchor, s.follow = false, true
	s.history = textHistory{} // as upstream setText: a new value starts a new history
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
	s.hasAnchor, s.follow = false, true
}

// LineCount returns the number of lines in a Textarea (at least 1), or 0
// for other node types.
func (n *Node) LineCount() int {
	if n.textarea == nil {
		return 0
	}
	return len(n.textarea.lines)
}

// insertTextarea inserts text at the cursor, replacing the selection
// (typing or pasting over it) as one undo step.
func (n *Node) insertTextarea(text string, kind editKind) bool {
	s := n.textarea
	text = sanitizeTextarea(text)
	if text == "" {
		return false
	}
	at := textPos{s.line, s.col}
	to := at
	if sel, ok := s.selectionRange(); ok {
		at, to, kind = textPos{sel.sl, sel.sc}, textPos{sel.el, sel.ec}, editOther
	}
	if !s.replaceRange(at, to, text, kind) {
		return false
	}
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
		used += clusterWidth(g.Str(), g.Width())
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

// editTextareaKey applies one key to the text and cursor, ignoring any
// selection; handleTextareaKey (textarea_select.go) wraps it.
func (n *Node) editTextareaKey(key *KeyEvent) {
	s := n.textarea
	line := s.lines[s.line]
	edited, handled := false, true
	vertical := false
	meta := key.Alt || key.Meta
	here := textPos{s.line, s.col}
	lastLine := len(s.lines) - 1
	lineEnd := func(i int) textPos { return textPos{i, len(s.lines[i])} }
	// cut deletes [a, b); an empty range means there is nothing to do.
	cut := func(a, b textPos, kind editKind) {
		edited = s.replaceRange(a, b, "", kind)
		handled = edited
	}

	switch {
	case (key.Name == "return" || key.Name == "kpenter") && meta:
		handled = n.submitTextarea()
	case key.Name == "return" || key.Name == "kpenter" || key.Name == "linefeed":
		handled = n.insertTextarea("\n", editOther)
	case key.Name == "up", key.Name == "down":
		vertical = true
		n.moveVertical(key.Name == "up")
	case isWordLeftKey(key):
		switch {
		case s.col > 0:
			s.col = prevWordBoundary(line, s.col)
		case s.line > 0:
			s.line--
			s.col = len(s.lines[s.line])
		default:
			handled = false
		}
	case isWordRightKey(key):
		switch {
		case s.col < len(line):
			s.col = nextWordBoundary(line, s.col)
		case s.line < lastLine:
			s.line, s.col = s.line+1, 0
		default:
			handled = false
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
		case s.line < lastLine:
			s.line++
			s.col = 0
		default:
			handled = false
		}
	case key.Name == "home":
		s.line, s.col = 0, 0
	case key.Name == "end":
		s.line = lastLine
		s.col = len(s.lines[s.line])
	case key.Ctrl && key.Name == "a":
		s.col = 0
	case key.Ctrl && key.Name == "e":
		s.col = len(line)
	case isDeleteWordBackKey(key):
		if s.col == 0 && s.line > 0 {
			cut(lineEnd(s.line-1), here, editOther)
		} else {
			cut(textPos{s.line, prevWordBoundary(line, s.col)}, here, editOther)
		}
	case isDeleteWordForwardKey(key):
		if s.col == len(line) && s.line < lastLine {
			cut(here, textPos{s.line + 1, 0}, editOther)
		} else {
			cut(here, textPos{s.line, nextWordBoundary(line, s.col)}, editOther)
		}
	case key.Name == "backspace":
		switch {
		case s.col > 0:
			cut(textPos{s.line, inputPrev(line, s.col)}, here, editBack)
		case s.line > 0:
			cut(lineEnd(s.line-1), here, editBack)
		default:
			handled = false
		}
	case key.Name == "delete" || (key.Ctrl && key.Name == "d"):
		switch {
		case s.col < len(line):
			cut(here, textPos{s.line, inputNext(line, s.col)}, editDelete)
		case s.line < lastLine:
			cut(here, textPos{s.line + 1, 0}, editDelete)
		default:
			handled = false
		}
	case key.Ctrl && key.Name == "k":
		if s.col < len(line) {
			cut(here, lineEnd(s.line), editOther)
		} else if s.line < lastLine {
			cut(here, textPos{s.line + 1, 0}, editOther)
		} else {
			handled = false
		}
	case key.Ctrl && key.Name == "u":
		cut(textPos{s.line, 0}, here, editOther)
	case !key.Ctrl && !meta && !key.Super && printableSequence(key.Sequence):
		handled = n.insertTextarea(key.Sequence, editType)
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

// clampTop keeps the scroll anchor inside the document after edits.
func (s *textareaState) clampTop() {
	s.topLine = min(max(s.topLine, 0), len(s.lines)-1)
	s.topRow = min(max(s.topRow, 0), len(s.rowsOf(s.topLine))-1)
}

func (s *textareaState) rowsOf(i int) []wrapRow {
	if s.opts.Wrap == WrapNone || s.wrapWidth <= 0 {
		return []wrapRow{{0, len(s.lines[i]), 0}}
	}
	return wrapLine(s.lines[i], s.wrapWidth, s.opts.Wrap)
}

// rowOf returns the visual row holding col. An offset equal to a row's end
// belongs to the next row (the cursor is shown at its start), except on
// the line's last row.
func rowOf(rows []wrapRow, col int) int {
	for k, row := range rows {
		if col < row.end {
			return k
		}
	}
	return len(rows) - 1
}

// moveVertical moves the cursor one visual row up or down, aiming at the
// remembered cell column within the row. At the top (bottom) it moves to
// the start (end) and forgets the goal column.
func (n *Node) moveVertical(up bool) {
	s := n.textarea
	rows := s.rowsOf(s.line)
	k := rowOf(rows, s.col)
	if s.goal < 0 {
		s.goal = displayWidth(string(s.lines[s.line][rows[k].start:s.col]))
	}
	line, row := s.line, k
	switch {
	case up && k > 0:
		row--
	case up && s.line > 0:
		line--
		rows = s.rowsOf(line)
		row = len(rows) - 1
	case up:
		s.col, s.goal = 0, -1
		return
	case k < len(rows)-1:
		row++
	case s.line < len(s.lines)-1:
		line++
		rows, row = s.rowsOf(line), 0
	default:
		s.col, s.goal = len(s.lines[s.line]), -1
		return
	}
	text := s.lines[line]
	r := rows[row]
	col := r.start + colAtCell(text[r.start:r.end], s.goal)
	if col == r.end && row < len(rows)-1 && col > r.start {
		col = inputPrev(text, col) // r.end would be drawn on the next row
	}
	s.line, s.col = line, col
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
	wrapping := s.opts.Wrap != WrapNone
	s.wrapWidth = 0
	if wrapping {
		s.wrapWidth, s.scrollX = max(r.W-1, 1), 0
	}
	s.clampTop()
	if len(s.lines) == 1 && len(s.lines[0]) == 0 && !n.focused {
		b.DrawText(s.opts.Placeholder, r.X, r.Y, s.opts.PlaceholderColor, bg, AttrDim)
		return
	}

	// Rows of the lines this paint touches; scrolling visits only lines
	// near the viewport, so cost follows its height, not the document.
	var cache map[int][]wrapRow
	var one [1]wrapRow // scratch for WrapNone: callers read a row at once
	rowsOf := func(i int) []wrapRow {
		if !wrapping {
			one[0] = wrapRow{0, len(s.lines[i]), 0}
			return one[:]
		}
		if cache == nil {
			cache = map[int][]wrapRow{}
		}
		rows, ok := cache[i]
		if !ok {
			rows = s.rowsOf(i)
			cache[i] = rows
		}
		return rows
	}
	// back moves a visual position up by count rows, stopping at the top.
	back := func(line, row, count int) (int, int) {
		for ; count > 0; count-- {
			if row > 0 {
				row--
			} else if line > 0 {
				line--
				row = len(rowsOf(line)) - 1
			} else {
				break
			}
		}
		return line, row
	}
	// ahead counts visual rows from (line, row) until (toLine, toRow) or
	// limit rows, whichever comes first; toLine < 0 means the end.
	ahead := func(line, row, toLine, toRow, limit int) (int, bool) {
		for count := 0; count < limit; count++ {
			if line == toLine && row == toRow {
				return count, true
			}
			if row++; row >= len(rowsOf(line)) {
				if line++; line >= len(s.lines) {
					return count + 1, false
				}
				row = 0
			}
		}
		return limit, false // the target is limit or more rows away
	}

	cur := s.lines[s.line]
	curRows := rowsOf(s.line)
	curRow := rowOf(curRows, s.col)
	// Keep the cursor's visual row inside the viewport (unless the wheel
	// moved the view away from it)...
	if !s.follow {
	} else if s.line < s.topLine || (s.line == s.topLine && curRow < s.topRow) {
		s.topLine, s.topRow = s.line, curRow
	} else if _, ok := ahead(s.topLine, s.topRow, s.line, curRow, r.H); !ok {
		s.topLine, s.topRow = back(s.line, curRow, r.H-1)
	}
	// ...and do not leave blank rows below the end when content shrinks.
	if count, _ := ahead(s.topLine, s.topRow, -1, 0, r.H); count < r.H {
		s.topLine, s.topRow = back(s.topLine, s.topRow, r.H-count)
	}

	// The cursor's cell within its row, and the glyph under it.
	cursorCell := displayWidth(string(cur[curRows[curRow].start:s.col]))
	glyphWidth := 1
	if s.col < len(cur) {
		glyphWidth = max(displayWidth(string(cur[s.col:inputNext(cur, s.col)])), 1)
	}
	if !wrapping && s.follow {
		if cursorCell < s.scrollX {
			s.scrollX = cursorCell
		} else if cursorCell+glyphWidth > s.scrollX+r.W {
			s.scrollX = cursorCell + glyphWidth - r.W
		}
		s.scrollX = max(s.scrollX, 0)
	}

	sel, selected := s.selectionRange()
	line, row, cursorY := s.topLine, s.topRow, -1
	for y := 0; y < r.H && line < len(s.lines); y++ {
		rows := rowsOf(line)
		span := rows[row]
		drawTextareaLine(b, s.lines[line][span.start:span.end], s.scrollX, r.W, r.X, r.Y+y, fg, bg)
		if selected {
			s.paintSelectedRow(b, sel, line, span, row == len(rows)-1, r.X-s.scrollX, r.Y+y)
		}
		if line == s.line && row == curRow {
			cursorY = y
		}
		if row++; row >= len(rows) {
			line, row = line+1, 0
		}
	}
	if n.focused && cursorY >= 0 {
		// Past the edge (after hanging spaces): show it on the last column.
		x := min(r.X+cursorCell-s.scrollX, r.X+r.W-1)
		b.drawCursor(x, r.Y+cursorY)
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
		w := clusterWidth(g.Str(), g.Width())
		if cell >= from && cell+w <= from+width {
			b.DrawText(g.Str(), x+cell-from, y, fg, bg, AttrNone)
		}
		cell += w
	}
}
