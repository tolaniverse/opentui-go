package opentui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// ---- Word boundaries --------------------------------------------------

// wordBreaks returns the rune offsets in line that follow a Unicode
// line-break opportunity (UAX #14), always ending with len(line). Word
// movement and word deletion stop at these offsets, as upstream's
// getNextWordBoundary/getPrevWordBoundary do: forward from "f|oo bar"
// lands after the space, at the start of "bar".
func wordBreaks(line []rune) []int {
	breaks := make([]int, 0, 8)
	str, state, pos := string(line), -1, 0
	for len(str) > 0 {
		var c string
		var boundaries int
		c, str, boundaries, state = uniseg.StepString(str, state)
		pos += utf8.RuneCountInString(c)
		if boundaries&uniseg.MaskLine != uniseg.LineDontBreak {
			breaks = append(breaks, pos)
		}
	}
	if len(breaks) == 0 || breaks[len(breaks)-1] != len(line) {
		breaks = append(breaks, len(line))
	}
	return breaks
}

// nextWordBoundary is the first break after col (the line end if none).
func nextWordBoundary(line []rune, col int) int {
	for _, b := range wordBreaks(line) {
		if b > col {
			return b
		}
	}
	return len(line)
}

// prevWordBoundary is the last break before col, or 0. (Upstream moves to
// the previous line from inside a line's first word; here it stops at the
// start of the line first.)
func prevWordBoundary(line []rune, col int) int {
	prev := 0
	for _, b := range wordBreaks(line) {
		if b >= col {
			break
		}
		prev = b
	}
	return prev
}

func isWordLeftKey(k *KeyEvent) bool {
	meta := k.Alt || k.Meta
	return (meta && (k.Name == "b" || k.Name == "left")) || (k.Ctrl && k.Name == "left")
}

func isWordRightKey(k *KeyEvent) bool {
	meta := k.Alt || k.Meta
	return (meta && (k.Name == "f" || k.Name == "right")) || (k.Ctrl && k.Name == "right")
}

func isDeleteWordBackKey(k *KeyEvent) bool {
	return (k.Ctrl && k.Name == "w") || ((k.Ctrl || k.Alt || k.Meta) && k.Name == "backspace")
}

func isDeleteWordForwardKey(k *KeyEvent) bool {
	meta := k.Alt || k.Meta
	return (meta && k.Name == "d") || ((meta || k.Ctrl) && k.Name == "delete")
}

// isUndoKey matches upstream's Ctrl+- and Super+Z, plus Ctrl+_ (what most
// terminals send for Ctrl+-) and Ctrl+Z, a port extension because Ctrl+-
// and Super+Z need the kitty keyboard protocol.
func isUndoKey(k *KeyEvent) bool {
	return (k.Ctrl && !k.Shift && (k.Name == "-" || k.Name == "_" || k.Name == "z")) ||
		(k.Super && !k.Shift && k.Name == "z")
}

// isRedoKey matches upstream's Ctrl+. and Super+Shift+Z, plus Ctrl+Y and
// Ctrl+Shift+Z.
func isRedoKey(k *KeyEvent) bool {
	return (k.Ctrl && (k.Name == "." || k.Name == "y" || (k.Shift && k.Name == "z"))) ||
		(k.Super && k.Shift && k.Name == "z")
}

// ---- Edit kinds and grouping -----------------------------------------

// editKind classifies an edit for undo grouping: consecutive edits of the
// same kind at the cursor merge into one undo step.
type editKind uint8

const (
	editOther  editKind = iota // never merges
	editType                   // a typed key
	editBack                   // Backspace of one cluster
	editDelete                 // Delete of one cluster
)

// historyLimit caps undo steps per editor.
const historyLimit = 500

// startsWord reports whether typing text after prev starts a new undo
// group: a space typed after a non-space ends the word being typed.
func startsWord(prev, text string) bool {
	if prev == "" || text == "" {
		return false
	}
	p, _ := utf8.DecodeLastRuneInString(prev)
	t, _ := utf8.DecodeRuneInString(text)
	return unicode.IsSpace(t) && !unicode.IsSpace(p)
}

// ---- Textarea history -------------------------------------------------

type textPos struct{ line, col int }

// after returns the position just past text inserted at a.
func (a textPos) after(text string) textPos {
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		return textPos{a.line + strings.Count(text, "\n"), utf8.RuneCountInString(text[i+1:])}
	}
	return textPos{a.line, a.col + utf8.RuneCountInString(text)}
}

// cursorState is what undo restores around an edit: cursor and selection.
type cursorState struct {
	pos       textPos
	hasAnchor bool
	anchor    textPos
}

// textEdit replaced removed with inserted at at. Undo and redo replay it
// in reverse or forward, so history costs the size of the edits, not of
// the document.
type textEdit struct {
	at                textPos
	removed, inserted string
	before, after     cursorState
	kind              editKind
}

type textHistory struct {
	undo, redo []textEdit
}

func (s *textareaState) cursorState() cursorState {
	return cursorState{textPos{s.line, s.col}, s.hasAnchor, textPos{s.anchorLine, s.anchorCol}}
}

func (s *textareaState) restore(c cursorState) {
	s.line, s.col = c.pos.line, c.pos.col
	s.hasAnchor, s.anchorLine, s.anchorCol = c.hasAnchor, c.anchor.line, c.anchor.col
	s.goal, s.follow = -1, true
}

func (s *textareaState) textBetween(a, b textPos) string {
	if a.line == b.line {
		return string(s.lines[a.line][a.col:b.col])
	}
	var out strings.Builder
	out.WriteString(string(s.lines[a.line][a.col:]))
	for i := a.line + 1; i < b.line; i++ {
		out.WriteByte('\n')
		out.WriteString(string(s.lines[i]))
	}
	out.WriteByte('\n')
	out.WriteString(string(s.lines[b.line][:b.col]))
	return out.String()
}

// applyRange replaces [a, b) with text (which may contain '\n') and
// returns the position after the inserted text. It records nothing.
func (s *textareaState) applyRange(a, b textPos, text string) textPos {
	prefix, suffix := s.lines[a.line][:a.col], s.lines[b.line][b.col:]
	if a.line == b.line && !strings.Contains(text, "\n") {
		// Typing and single-line deletes: rebuild one line only.
		insert := []rune(text)
		line := make([]rune, 0, len(prefix)+len(insert)+len(suffix))
		line = append(append(append(line, prefix...), insert...), suffix...)
		s.lines[a.line] = line
		return textPos{a.line, a.col + len(insert)}
	}
	parts := strings.Split(text, "\n")
	added := make([][]rune, len(parts))
	for i, p := range parts {
		added[i] = []rune(p)
	}
	added[0] = append(append([]rune(nil), prefix...), added[0]...)
	last := len(added) - 1
	end := textPos{a.line + last, len(added[last])}
	added[last] = append(added[last], suffix...)
	lines := make([][]rune, 0, len(s.lines)-(b.line-a.line)+last)
	lines = append(lines, s.lines[:a.line]...)
	lines = append(lines, added...)
	lines = append(lines, s.lines[b.line+1:]...)
	s.lines = lines
	return end
}

// replaceRange is the one way Textarea text changes: it replaces [a, b)
// with text, places the cursor after it (or at a for a deletion), clears
// the selection, and records an undo step. It reports whether anything
// changed; the caller notifies OnInput.
func (s *textareaState) replaceRange(a, b textPos, text string, kind editKind) bool {
	removed := s.textBetween(a, b)
	if removed == "" && text == "" {
		return false
	}
	before := s.cursorState()
	end := s.applyRange(a, b, text)
	if text != "" {
		// Inserted combining marks can merge with what follows.
		s.line, s.col = end.line, inputCeil(s.lines[end.line], end.col)
	} else {
		// A deletion can join clusters across the cut.
		s.line, s.col = a.line, inputFloor(s.lines[a.line], a.col)
	}
	s.hasAnchor, s.goal, s.follow = false, -1, true
	s.record(textEdit{at: a, removed: removed, inserted: text, before: before, after: s.cursorState(), kind: kind})
	return true
}

func (s *textareaState) record(e textEdit) {
	h := &s.history
	h.redo = h.redo[:0]
	if n := len(h.undo); n > 0 && e.kind != editOther {
		top := &h.undo[n-1]
		if top.kind == e.kind && top.after.pos == e.before.pos && !e.before.hasAnchor {
			switch e.kind {
			case editType:
				// Contiguous only: after typing before a combining mark the
				// cursor skips past the mark, away from the insertion's end.
				if e.at == top.at.after(top.inserted) && top.removed == "" &&
					!startsWord(top.inserted, e.inserted) && !strings.Contains(e.inserted, "\n") {
					top.inserted += e.inserted
					top.after = e.after
					return
				}
			case editBack:
				if e.at.after(e.removed) == top.at {
					top.at, top.removed, top.after = e.at, e.removed+top.removed, e.after
					return
				}
			case editDelete:
				if e.at == top.at {
					top.removed += e.removed
					top.after = e.after
					return
				}
			}
		}
	}
	h.undo = append(h.undo, e)
	if len(h.undo) > historyLimit {
		h.undo = append(h.undo[:0], h.undo[len(h.undo)-historyLimit:]...)
	}
}

func (s *textareaState) undo() bool {
	h := &s.history
	if len(h.undo) == 0 {
		return false
	}
	e := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	s.applyRange(e.at, e.at.after(e.inserted), e.removed)
	s.restore(e.before)
	h.redo = append(h.redo, e)
	return true
}

func (s *textareaState) redoEdit() bool {
	h := &s.history
	if len(h.redo) == 0 {
		return false
	}
	e := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	s.applyRange(e.at, e.at.after(e.removed), e.inserted)
	s.restore(e.after)
	h.undo = append(h.undo, e)
	return true
}

// ---- Input history ----------------------------------------------------

// inputSnapshot is one undo step of an Input. An Input holds at most
// MaxLength clusters, so whole-value snapshots stay small.
type inputSnapshot struct {
	value  string
	cursor int
}

type inputHistory struct {
	undo, redo []inputSnapshot
	lastKind   editKind
	lastCursor int    // cursor after the last edit, for grouping
	lastText   string // text of the last typed group, for word grouping
}

// checkpoint saves the value before an edit unless the edit continues the
// current group (same kind, at the cursor where the last edit left it).
func (n *Node) inputCheckpoint(kind editKind, text string) {
	s := n.input
	h := &s.history
	merge := kind != editOther && kind == h.lastKind && s.cursor == h.lastCursor && len(h.undo) > 0 &&
		!(kind == editType && startsWord(h.lastText, text))
	if !merge {
		h.undo = append(h.undo, inputSnapshot{string(s.value), s.cursor})
		if len(h.undo) > historyLimit {
			h.undo = append(h.undo[:0], h.undo[len(h.undo)-historyLimit:]...)
		}
		h.lastText = ""
	}
	h.redo = h.redo[:0]
	h.lastKind = kind
	h.lastText += text
}

// inputEdited records where the cursor ended after an edit.
func (n *Node) inputEdited() { n.input.history.lastCursor = n.input.cursor }

func (n *Node) inputRestore(from, to *[]inputSnapshot) bool {
	s := n.input
	if len(*from) == 0 {
		return false
	}
	snap := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, inputSnapshot{string(s.value), s.cursor})
	s.value, s.cursor = []rune(snap.value), snap.cursor
	s.history.lastKind = editOther
	n.notifyInput()
	return true
}

// ---- Public API -------------------------------------------------------

// Undo reverts the last edit group of an Input or Textarea, restoring the
// cursor and selection from before it, and reports whether anything was
// undone. Typing merges into one step per word; runs of Backspace or
// Delete merge too. SetValue clears the history, as upstream's setText.
func (n *Node) undo() bool {
	switch {
	case n.textarea != nil:
		if n.textarea.undo() {
			n.notifyTextarea()
			return true
		}
	case n.input != nil:
		return n.inputRestore(&n.input.history.undo, &n.input.history.redo)
	}
	return false
}

// Redo reapplies the last undone edit group; any new edit clears it.
func (n *Node) redo() bool {
	switch {
	case n.textarea != nil:
		if n.textarea.redoEdit() {
			n.notifyTextarea()
			return true
		}
	case n.input != nil:
		return n.inputRestore(&n.input.history.redo, &n.input.history.undo)
	}
	return false
}
