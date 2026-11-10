package opentui

import (
	"strings"
	"testing"
)

func typeKeys(handle NodeHandle, text string) {
	n := handle.AsNode()
	for _, r := range text {
		n.keyHandler(key(string(r), string(r)))
	}
}

func superKey(name string, shift bool) *KeyEvent {
	k := press(name, name)
	k.Super, k.Shift = true, shift
	return k
}

func TestTextareaUndoGroupsTypingByWord(t *testing.T) {
	n := Textarea(TextareaOptions{})
	typeKeys(n, "hello world")
	for _, want := range []string{"hello", ""} {
		if !n.Undo() || n.Value() != want {
			t.Fatalf("undo: %q, want %q", n.Value(), want)
		}
	}
	if n.Undo() {
		t.Fatal("undo past the beginning")
	}
	for _, want := range []string{"hello", "hello world"} {
		if !n.Redo() || n.Value() != want {
			t.Fatalf("redo: %q, want %q", n.Value(), want)
		}
	}
	if n.Redo() {
		t.Fatal("redo past the end")
	}
	cursorAt(t, n, 0, 11)
}

func TestTextareaUndoBackspaceAndDeleteRuns(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "abcdef"})
	for i := 0; i < 3; i++ {
		n.keyHandler(key("backspace", "\x7f"))
	}
	n.SetCursorPosition(0, 0)
	n.keyHandler(key("delete", "\x1b[3~"))
	n.keyHandler(key("delete", "\x1b[3~"))
	if got := n.Value(); got != "c" {
		t.Fatalf("after edits: %q", got)
	}
	n.Undo() // both Deletes at once
	if got := n.Value(); got != "abc" {
		t.Fatalf("undo delete run: %q", got)
	}
	cursorAt(t, n, 0, 0)
	n.Undo() // all three Backspaces at once
	if got := n.Value(); got != "abcdef" {
		t.Fatalf("undo backspace run: %q", got)
	}
	cursorAt(t, n, 0, 6)
}

func TestTextareaUndoRestoresSelectionAndLines(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "hello world\nnext"})
	n.SetCursorPosition(0, 6)
	n.keyHandler(key("e", "\x05", "ctrl", "shift"))
	n.keyHandler(key("X", "X")) // replaces "world"
	if got := n.Value(); got != "hello X\nnext" {
		t.Fatalf("typed over selection: %q", got)
	}
	n.Undo()
	if got, sel := n.Value(), n.SelectedText(); got != "hello world\nnext" || sel != "world" {
		t.Fatalf("undo: %q, selection %q", got, sel)
	}
	// Backspace at a line start joins lines; undo splits them again.
	n.SetCursorPosition(1, 0)
	n.keyHandler(key("backspace", "\x7f"))
	n.Undo()
	if n.LineCount() != 2 {
		t.Fatalf("undo of a join: %q", n.Value())
	}
	cursorAt(t, n, 1, 0)
	// A multi-line paste is one step.
	n.InsertText("a\nb\nc")
	n.Undo()
	if got := n.Value(); got != "hello world\nnext" {
		t.Fatalf("undo paste: %q", got)
	}
	n.Redo()
	if got := n.Value(); got != "hello world\na\nb\ncnext" {
		t.Fatalf("redo paste: %q", got)
	}
}

func TestTextareaUndoGroupingBreaksAndRedoClears(t *testing.T) {
	n := Textarea(TextareaOptions{})
	typeKeys(n, "ab")
	n.keyHandler(key("left", "\x1b[D"))
	typeKeys(n, "c")
	if got := n.Value(); got != "acb" {
		t.Fatalf("value %q", got)
	}
	n.Undo() // moving the cursor started a new group
	if got := n.Value(); got != "ab" {
		t.Fatalf("undo after a move: %q", got)
	}
	typeKeys(n, "z")
	if n.Redo() {
		t.Fatal("a new edit must clear redo")
	}
	n.SetValue("fresh")
	if n.Undo() {
		t.Fatal("SetValue must clear the history")
	}
}

func TestUndoRedoKeyBindings(t *testing.T) {
	undo := []*KeyEvent{key("z", "\x1a", "ctrl"), key("_", "\x1f", "ctrl"), key("-", "-", "ctrl"), superKey("z", false)}
	redo := []*KeyEvent{key("y", "\x19", "ctrl"), key(".", ".", "ctrl"), key("z", "z", "ctrl", "shift"), superKey("z", true)}
	for i := range undo {
		for _, n := range []NodeHandle{Textarea(TextareaOptions{}), Input(InputOptions{})} {
			node := n.AsNode()
			node.insertText("x")
			u := *undo[i]
			node.keyHandler(&u)
			if node.value() != "" || !u.DefaultPrevented() {
				t.Fatalf("undo key %d (%q): value %q", i, undo[i].Sequence, node.value())
			}
			r := *redo[i]
			node.keyHandler(&r)
			if node.value() != "x" || !r.DefaultPrevented() {
				t.Fatalf("redo key %d (%q): value %q", i, redo[i].Sequence, node.value())
			}
		}
	}
}

func TestTextareaHistoryLimit(t *testing.T) {
	n := Textarea(TextareaOptions{})
	for i := 0; i < historyLimit+100; i++ {
		n.keyHandler(key("return", "\r")) // each newline is its own step
	}
	undone := 0
	for n.Undo() {
		undone++
	}
	if undone != historyLimit || n.LineCount() != 101 {
		t.Fatalf("undid %d steps leaving %d lines, want %d and 101", undone, n.LineCount(), historyLimit)
	}
}

func TestWordMovementFollowsLineBreakOpportunities(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "foo bar  baz\nnext"})
	n.SetCursorPosition(0, 0)
	for _, want := range []int{4, 9, 12} {
		n.keyHandler(key("right", "\x1b[1;5C", "ctrl"))
		cursorAt(t, n, 0, want)
	}
	n.keyHandler(key("f", "\x1bf", "meta")) // line end: next line
	cursorAt(t, n, 1, 0)
	n.keyHandler(key("b", "\x1bb", "meta")) // line start: previous line end
	cursorAt(t, n, 0, 12)
	for _, want := range []int{9, 4, 0} {
		n.keyHandler(key("left", "\x1b[1;3D", "meta"))
		cursorAt(t, n, 0, want)
	}
	n.keyHandler(key("right", "\x1b[1;6C", "ctrl", "shift"))
	if got := n.SelectedText(); got != "foo " {
		t.Fatalf("shift+ctrl+right = %q", got)
	}
	// Ideographs are each a break opportunity.
	m := Textarea(TextareaOptions{Value: "abc 日本"})
	m.SetCursorPosition(0, 0)
	for _, want := range []int{4, 5, 6} {
		m.keyHandler(key("right", "\x1b[1;5C", "ctrl"))
		cursorAt(t, m, 0, want)
	}
}

func TestWordDeletion(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "foo bar\nbaz"})
	n.SetCursorPosition(0, 0)
	n.keyHandler(key("d", "\x1bd", "meta"))
	if got := n.Value(); got != "bar\nbaz" {
		t.Fatalf("meta+d: %q", got)
	}
	n.SetCursorPosition(0, 3)
	n.keyHandler(key("delete", "\x1b[3;5~", "ctrl")) // at line end: join
	if got := n.Value(); got != "barbaz" {
		t.Fatalf("ctrl+delete at line end: %q", got)
	}
	n.keyHandler(key("backspace", "\x1b\x7f", "meta"))
	if got := n.Value(); got != "baz" {
		t.Fatalf("meta+backspace: %q", got)
	}
	n.Undo()
	if got := n.Value(); got != "barbaz" {
		t.Fatalf("undo word delete: %q", got)
	}
}

func TestInputUndoAndWordKeys(t *testing.T) {
	n := Input(InputOptions{})
	typeKeys(n, "hello big world")
	n.keyHandler(key("left", "\x1b[1;5D", "ctrl"))
	if n.Cursor() != 10 {
		t.Fatalf("ctrl+left: cursor %d, want 10", n.Cursor())
	}
	n.keyHandler(key("w", "\x17", "ctrl"))
	if got := n.Value(); got != "hello world" {
		t.Fatalf("ctrl+w: %q", got)
	}
	n.keyHandler(key("d", "\x1bd", "meta"))
	if got := n.Value(); got != "hello " {
		t.Fatalf("meta+d: %q", got)
	}
	for _, want := range []string{"hello world", "hello big world", "hello big", "hello", ""} {
		if !n.Undo() || n.Value() != want {
			t.Fatalf("undo: %q, want %q", n.Value(), want)
		}
	}
	n.Redo()
	if got := n.Value(); got != "hello" {
		t.Fatalf("redo: %q", got)
	}
	n.SetValue("new")
	if n.Undo() {
		t.Fatal("SetValue must clear Input history")
	}
}

// undoRoundTrip checks the history after a random edit session: undoing
// everything restores the initial value, and redoing as many steps
// restores the final one.
func undoRoundTrip(t *testing.T, n *Node, initial string) {
	t.Helper()
	final := n.value()
	steps := 0
	for n.undo() {
		if steps++; steps > historyLimit {
			t.Fatal("undo did not terminate")
		}
	}
	if got := n.value(); got != initial {
		t.Fatalf("undo all: %q, want initial %q (final %q)", got, initial, final)
	}
	for i := 0; i < steps; i++ {
		if !n.redo() {
			t.Fatalf("redo %d of %d failed", i+1, steps)
		}
	}
	if got := n.value(); got != final {
		t.Fatalf("redo all: %q, want final %q", got, final)
	}
}

var historyFuzzKeys = []*KeyEvent{
	key("left", "\x1b[D"), key("right", "\x1b[C"), key("up", "\x1b[A"), key("down", "\x1b[B"),
	key("return", "\r"), key("backspace", "\x7f"), key("delete", "\x1b[3~"),
	key("x", "x"), key(" ", " "), key("界", "界"), key("", "́"),
	key("w", "\x17", "ctrl"), key("k", "\x0b", "ctrl"), key("u", "\x15", "ctrl"),
	key("d", "\x1bd", "meta"), key("right", "\x1b[1;5C", "ctrl"), key("left", "\x1b[1;5D", "ctrl"),
	key("right", "\x1b[1;2C", "shift"), key("end", "\x1b[1;2F", "shift"),
	key("z", "\x1a", "ctrl"), key("y", "\x19", "ctrl"),
}

func FuzzEditorHistory(f *testing.F) {
	f.Add("hello world\nfoo 界 bar", []byte{7, 8, 7, 5, 0, 4, 19, 19, 20, 17, 18, 7, 14, 11, 12, 13})
	f.Add("", []byte{7, 7, 8, 7, 19, 20, 20, 4, 9, 10, 5, 5, 19})
	f.Fuzz(func(t *testing.T, value string, ops []byte) {
		ops = ops[:min(len(ops), 300)]
		for _, n := range []*Node{Textarea(TextareaOptions{Value: value}).Node, Input(InputOptions{Value: value}).Node} {
			initial := n.value()
			for _, op := range ops {
				k := *historyFuzzKeys[int(op)%len(historyFuzzKeys)]
				n.keyHandler(&k)
			}
			if n.textarea != nil && strings.Count(n.value(), "\n")+1 != n.lineCount() {
				t.Fatalf("line count %d for %q", n.lineCount(), n.value())
			}
			undoRoundTrip(t, n, initial)
		}
	})
}

// Typing before a lone combining mark merges the typed rune with it, so
// the cursor skips past the mark; the next typed rune must not join the
// same undo step, or undo would remove the mark instead of the rune.
func TestTextareaUndoTypingBeforeCombiningMark(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "́b"})
	n.SetCursorPosition(0, 0)
	typeKeys(n, "xy")
	for n.Undo() {
	}
	if got := n.Value(); got != "́b" {
		t.Fatalf("undo all = %q, want %q", got, "́b")
	}
}
