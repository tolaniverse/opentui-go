package opentui

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// selectionState is a Textarea's selection: from the anchor to the cursor.
// It is part of the editor, separate from the renderer's Text selection.
type selectionState struct {
	hasAnchor             bool
	anchorLine, anchorCol int

	dragging           bool // a left press started here and is held
	lastClick          time.Time
	lastLine, lastCol  int // position of the last press, for double-click
	lastClickWasDouble bool
}

// textRange is an ordered span of positions: (sl, sc) up to (el, ec).
type textRange struct{ sl, sc, el, ec int }

// selectionRange returns the ordered selection, or false when there is no
// anchor or the anchor equals the cursor.
func (s *textareaState) selectionRange() (textRange, bool) {
	if !s.hasAnchor || (s.anchorLine == s.line && s.anchorCol == s.col) {
		return textRange{}, false
	}
	r := textRange{s.anchorLine, s.anchorCol, s.line, s.col}
	if r.sl > r.el || (r.sl == r.el && r.sc > r.ec) {
		r = textRange{r.el, r.ec, r.sl, r.sc}
	}
	return r, true
}

func (s *textareaState) selectedText() string {
	r, ok := s.selectionRange()
	if !ok {
		return ""
	}
	if r.sl == r.el {
		return string(s.lines[r.sl][r.sc:r.ec])
	}
	var b strings.Builder
	b.WriteString(string(s.lines[r.sl][r.sc:]))
	for i := r.sl + 1; i < r.el; i++ {
		b.WriteByte('\n')
		b.WriteString(string(s.lines[i]))
	}
	b.WriteByte('\n')
	b.WriteString(string(s.lines[r.el][:r.ec]))
	return b.String()
}

// deleteSelection removes the selected text as one undo step, leaving the
// cursor where it began, and reports whether anything was removed. It
// does not notify.
func (s *textareaState) deleteSelection() bool {
	r, ok := s.selectionRange()
	s.hasAnchor = false
	if !ok {
		return false
	}
	return s.replaceRange(textPos{r.sl, r.sc}, textPos{r.el, r.ec}, "", editOther)
}

// selectedText returns a Textarea's selected text ("\n" between lines), or
// "" when nothing is selected. For the renderer's
// selection of Text nodes, use Renderer.SelectedText.
func (n *Node) selectedText() string {
	if n.textarea == nil {
		return ""
	}
	return n.textarea.selectedText()
}

// SelectAll selects a Textarea's whole value, with the cursor at the end.
func (n *Node) selectAll() {
	if n.textarea == nil {
		return
	}
	s := n.textarea
	s.hasAnchor, s.anchorLine, s.anchorCol = true, 0, 0
	s.line = len(s.lines) - 1
	s.col, s.goal, s.follow = len(s.lines[s.line]), -1, true
}

func isMovementKey(k *KeyEvent) bool {
	switch k.Name {
	case "left", "right", "up", "down", "home", "end":
		return true
	case "a", "e":
		return k.Ctrl
	case "b", "f":
		return k.Ctrl || k.Alt || k.Meta
	}
	return false
}

func isDeleteKey(k *KeyEvent) bool {
	switch k.Name {
	case "backspace", "delete":
		return true
	case "d":
		return k.Ctrl || k.Alt || k.Meta
	case "w", "k", "u":
		return k.Ctrl
	}
	return false
}

// handleTextareaKey adds selection to editTextareaKey. Shift with any
// movement key extends the selection from the anchor; plain Left/Right
// collapse it to its start/end; a delete key removes it; typing and paste
// replace it (insertTextarea); Super+A selects all, as upstream binds it.
func (n *Node) handleTextareaKey(key *KeyEvent) {
	if n.textarea == nil || key.Type == KeyRelease {
		return
	}
	s := n.textarea
	s.follow = true
	sel, selected := s.selectionRange()
	switch {
	case isUndoKey(key) || isRedoKey(key):
		if isUndoKey(key) {
			n.undo()
		} else {
			n.redo()
		}
		key.PreventDefault() // also when there is nothing to undo
		return
	case key.Super && key.Name == "a":
		n.selectAll()
		key.PreventDefault()
		return
	case isMovementKey(key) && key.Shift:
		if !s.hasAnchor {
			s.hasAnchor, s.anchorLine, s.anchorCol = true, s.line, s.col
		}
		n.editTextareaKey(key)
		return
	case isMovementKey(key) && selected && !isWordLeftKey(key) && (key.Name == "left" || key.Name == "b"):
		s.line, s.col, s.hasAnchor, s.goal = sel.sl, sel.sc, false, -1
		key.PreventDefault()
		return
	case isMovementKey(key) && selected && !isWordRightKey(key) && (key.Name == "right" || key.Name == "f"):
		s.line, s.col, s.hasAnchor, s.goal = sel.el, sel.ec, false, -1
		key.PreventDefault()
		return
	case isDeleteKey(key) && selected:
		s.deleteSelection()
		n.notifyTextarea()
		key.PreventDefault()
		return
	case isMovementKey(key) || isDeleteKey(key):
		s.hasAnchor = false
	}
	n.editTextareaKey(key)
}

// rowStep moves a visual position by delta rows (negative is up), clamped
// to the document, and returns how many rows it actually moved.
func (s *textareaState) rowStep(line, row, delta int) (int, int, int) {
	moved := 0
	for ; delta < 0; delta++ {
		if row > 0 {
			row--
		} else if line > 0 {
			line--
			row = len(s.rowsOf(line)) - 1
		} else {
			break
		}
		moved--
	}
	for ; delta > 0; delta-- {
		if row+1 < len(s.rowsOf(line)) {
			row++
		} else if line+1 < len(s.lines) {
			line, row = line+1, 0
		} else {
			break
		}
		moved++
	}
	return line, row, moved
}

// textareaPosition maps a screen cell to a cursor position using the last
// painted layout. Points above or below the view land one row outside it,
// so dragging past an edge scrolls; points past the start or end of the
// document go to its first or last position.
func (n *Node) textareaPosition(x, y int) (line, col int) {
	s := n.textarea
	s.clampTop()
	r := n.rect.Inset(n.Style.Padding)
	dy := max(min(y-r.Y, r.H), -1)
	line, row, moved := s.rowStep(s.topLine, s.topRow, dy)
	switch {
	case moved < dy:
		return line, len(s.lines[line])
	case moved > dy:
		return 0, 0
	}
	rows := s.rowsOf(line)
	span := rows[row]
	text := s.lines[line]
	col = span.start + colAtCell(text[span.start:span.end], x-r.X+s.scrollX)
	if col == span.end && row < len(rows)-1 && col > span.start {
		col = inputPrev(text, col) // span.end is drawn on the next row
	}
	return line, col
}

// wordAt returns the rune range of the word (or run of non-word clusters)
// at col, by grapheme cluster.
func (s *textareaState) wordAt(line, col int) (int, int) {
	text := s.lines[line]
	offsets := inputOffsets(text)
	if len(offsets) < 2 {
		return 0, 0
	}
	i := len(offsets) - 2
	for k := 0; k+1 < len(offsets); k++ {
		if col < offsets[k+1] {
			i = k
			break
		}
	}
	isWord := func(k int) bool {
		r, _ := utf8.DecodeRuneInString(string(text[offsets[k]:offsets[k+1]]))
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	class := isWord(i)
	left, right := i, i
	for left > 0 && isWord(left-1) == class {
		left--
	}
	for right+2 < len(offsets) && isWord(right+1) == class {
		right++
	}
	return offsets[left], offsets[right+1]
}

const doubleClickInterval = 500 * time.Millisecond

func (n *Node) handleTextareaMouse(e *MouseEvent) {
	s := n.textarea
	switch e.Type {
	case MouseDown:
		if e.Button != MouseLeft {
			return
		}
		line, col := n.textareaPosition(e.X, e.Y)
		now := time.Now()
		double := !e.Modifiers.Shift && !s.lastClickWasDouble &&
			now.Sub(s.lastClick) <= doubleClickInterval && line == s.lastLine && col == s.lastCol
		switch {
		case e.Modifiers.Shift:
			if !s.hasAnchor {
				s.hasAnchor, s.anchorLine, s.anchorCol = true, s.line, s.col
			}
			s.line, s.col = line, col
		case double:
			start, end := s.wordAt(line, col)
			s.hasAnchor, s.anchorLine, s.anchorCol = true, line, start
			s.line, s.col = line, end
		default:
			s.hasAnchor, s.anchorLine, s.anchorCol = true, line, col
			s.line, s.col = line, col
		}
		s.lastClick, s.lastLine, s.lastCol, s.lastClickWasDouble = now, line, col, double
		s.dragging, s.follow, s.goal = true, true, -1
		e.StopPropagation()
	case MouseDrag:
		if !s.dragging {
			return
		}
		s.line, s.col = n.textareaPosition(e.X, e.Y)
		s.follow, s.goal = true, -1
		e.StopPropagation()
	case MouseUp:
		if !s.dragging {
			return
		}
		s.dragging = false
		if _, ok := s.selectionRange(); !ok {
			s.hasAnchor = false
		} else if s.opts.OnSelect != nil {
			s.opts.OnSelect(s.selectedText())
		}
		e.StopPropagation()
	case MouseScroll:
		if e.Scroll != nil && n.wheelTextarea(e.Scroll) {
			e.PreventDefault()
			e.StopPropagation()
		}
	}
}

// wheelRows is how many rows (or columns) one wheel notch scrolls, as
// ScrollBox's default WheelStep.
const wheelRows = 3

// wheelTextarea scrolls the view without moving the cursor and reports
// whether it moved; at an edge the event is left for a parent to handle.
func (n *Node) wheelTextarea(info *ScrollInfo) bool {
	s := n.textarea
	s.clampTop()
	r := n.rect.Inset(n.Style.Padding)
	delta := info.Delta * wheelRows
	switch info.Direction {
	case ScrollUp, ScrollDown:
		if info.Direction == ScrollUp {
			delta = -delta
		}
		line, row, moved := s.rowStep(s.topLine, s.topRow, delta)
		if moved > 0 { // do not scroll past the point where the end is at the bottom
			lastLine, lastRow, _ := s.rowStep(len(s.lines)-1, len(s.rowsOf(len(s.lines)-1))-1, 0)
			maxLine, maxRow, _ := s.rowStep(lastLine, lastRow, -(r.H - 1))
			if line > maxLine || (line == maxLine && row > maxRow) {
				line, row = maxLine, maxRow
			}
		}
		if line == s.topLine && row == s.topRow {
			return false
		}
		s.topLine, s.topRow, s.follow = line, row, false
		return true
	case ScrollLeft, ScrollRight:
		if s.opts.Wrap != WrapNone {
			return false
		}
		if info.Direction == ScrollLeft {
			delta = -delta
		}
		widest := 0
		for i := s.topLine; i < min(len(s.lines), s.topLine+r.H); i++ {
			widest = max(widest, displayWidth(string(s.lines[i]))+1)
		}
		x := min(max(s.scrollX+delta, 0), max(widest-r.W, 0))
		if x == s.scrollX {
			return false
		}
		s.scrollX, s.follow = x, false
		return true
	}
	return false
}

// paintSelectedRow recolors the selected cells of one visual row painted
// at y, whose cell 0 is at x0. A selection that continues past the end of
// the line also colors one cell after it, to show the line break.
func (s *textareaState) paintSelectedRow(b *Buffer, sel textRange, line int, span wrapRow, lastRow bool, x0, y int) {
	if line < sel.sl || line > sel.el {
		return
	}
	text := s.lines[line]
	from, to := span.start, span.end
	if line == sel.sl {
		from = max(from, sel.sc)
	}
	if line == sel.el {
		to = min(to, sel.ec)
	}
	fg, bg := s.opts.SelectionFG, s.opts.SelectionBG
	if from < to {
		x := x0 + displayWidth(string(text[span.start:from]))
		b.recolor(x, x+displayWidth(string(text[from:to])), y, fg, bg)
	}
	if lastRow && line < sel.el {
		x := x0 + displayWidth(string(text[span.start:span.end]))
		b.recolor(x, x+1, y, fg, bg)
	}
}
