package opentui

import (
	"strings"
	"testing"
)

func key(name, sequence string, mods ...string) *KeyEvent {
	k := press(name, sequence)
	for _, m := range mods {
		switch m {
		case "ctrl":
			k.Ctrl = true
		case "meta":
			k.Alt, k.Meta = true, true
		case "shift":
			k.Shift = true
		}
	}
	return k
}

func cursorAt(t *testing.T, n *Node, line, col int) {
	t.Helper()
	if l, c := n.CursorPosition(); l != line || c != col {
		t.Fatalf("cursor = (%d,%d), want (%d,%d); value %q", l, c, line, col, n.Value())
	}
}

func TestTextareaNewlineSplitsAndBackspaceJoins(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "ab"})
	cursorAt(t, n, 0, 2)
	n.SetCursorPosition(0, 1)
	n.keyHandler(key("return", "\r"))
	if got := n.Value(); got != "a\nb" {
		t.Fatalf("Enter: %q", got)
	}
	cursorAt(t, n, 1, 0)
	n.keyHandler(key("backspace", "\x7f"))
	if got := n.Value(); got != "ab" {
		t.Fatalf("Backspace at line start: %q", got)
	}
	cursorAt(t, n, 0, 1)
	n.SetValue("x\ny")
	n.SetCursorPosition(0, 1)
	n.keyHandler(key("delete", "\x1b[3~"))
	if got := n.Value(); got != "xy" {
		t.Fatalf("Delete at line end: %q", got)
	}
	cursorAt(t, n, 0, 1)
}

func TestTextareaVerticalMovementKeepsCellColumn(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "A界B\nxyzw\nq"})
	n.SetCursorPosition(1, 3) // terminal cell 3
	n.keyHandler(key("up", "\x1b[A"))
	cursorAt(t, n, 0, 2) // before B: A=cell 0, 界=cells 1-2, B=cell 3
	n.keyHandler(key("down", "\x1b[B"))
	cursorAt(t, n, 1, 3)
	n.keyHandler(key("down", "\x1b[B"))
	cursorAt(t, n, 2, 1) // clamped to the end of "q"
	n.keyHandler(key("up", "\x1b[A"))
	cursorAt(t, n, 1, 3) // the goal column survives the short line
	n.SetCursorPosition(1, 2)
	n.keyHandler(key("up", "\x1b[A"))
	cursorAt(t, n, 0, 1) // cell 2 is inside 界: snap left to its start
	n.keyHandler(key("up", "\x1b[A"))
	cursorAt(t, n, 0, 0) // up on the first line goes to its start
	n.SetCursorPosition(2, 0)
	n.keyHandler(key("down", "\x1b[B"))
	cursorAt(t, n, 2, 1) // down on the last line goes to its end
}

func TestTextareaHorizontalMovementCrossesLines(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "a\nb"})
	n.SetCursorPosition(1, 0)
	n.keyHandler(key("left", "\x1b[D"))
	cursorAt(t, n, 0, 1)
	n.keyHandler(key("right", "\x1b[C"))
	cursorAt(t, n, 1, 0)
	n.keyHandler(key("b", "\x02", "ctrl"))
	cursorAt(t, n, 0, 1)
	n.keyHandler(key("f", "\x06", "ctrl"))
	cursorAt(t, n, 1, 0)
}

func TestTextareaLineAndBufferKeys(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "one two\nthree four"})
	n.SetCursorPosition(1, 5)
	n.keyHandler(key("a", "\x01", "ctrl"))
	cursorAt(t, n, 1, 0)
	n.keyHandler(key("e", "\x05", "ctrl"))
	cursorAt(t, n, 1, 10)
	n.keyHandler(key("home", "\x1b[H"))
	cursorAt(t, n, 0, 0)
	n.keyHandler(key("end", "\x1b[F"))
	cursorAt(t, n, 1, 10)

	n.SetCursorPosition(1, 5)
	n.keyHandler(key("k", "\x0b", "ctrl"))
	if got := n.Value(); got != "one two\nthree" {
		t.Fatalf("Ctrl+K: %q", got)
	}
	n.keyHandler(key("u", "\x15", "ctrl"))
	if got := n.Value(); got != "one two\n" {
		t.Fatalf("Ctrl+U: %q", got)
	}
	cursorAt(t, n, 1, 0)
	n.keyHandler(key("w", "\x17", "ctrl")) // at line start: removes the newline
	if got := n.Value(); got != "one two" {
		t.Fatalf("Ctrl+W at line start: %q", got)
	}
	n.keyHandler(key("w", "\x17", "ctrl"))
	if got := n.Value(); got != "one " {
		t.Fatalf("Ctrl+W word: %q", got)
	}
}

func TestTextareaPasteNormalizesLineEndingsAndControls(t *testing.T) {
	n := Textarea(TextareaOptions{})
	n.InsertText("x\r\ny\rz\x1b[31m\tq")
	if got := n.Value(); got != "x\ny\nz[31m    q" {
		t.Fatalf("paste: %q", got)
	}
	cursorAt(t, n, 2, 10)
	if n.LineCount() != 3 {
		t.Fatalf("LineCount = %d", n.LineCount())
	}
}

func TestTextareaGraphemeEditing(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "Aé\n👩🏽‍💻Z"})
	n.SetCursorPosition(1, 0)
	n.keyHandler(key("left", "\x1b[D"))
	cursorAt(t, n, 0, 3)
	n.keyHandler(key("backspace", "\x7f"))
	if got := n.Value(); got != "A\n👩🏽‍💻Z" {
		t.Fatalf("backspace of combining cluster: %q", got)
	}
	n.SetCursorPosition(1, 0)
	n.keyHandler(key("delete", "\x1b[3~"))
	if got := n.Value(); got != "A\nZ" {
		t.Fatalf("delete of emoji cluster: %q", got)
	}
	n.SetCursorPosition(0, 99)
	cursorAt(t, n, 0, 1)
}

func TestTextareaSubmitChangeAndInputCallbacks(t *testing.T) {
	var inputs, changes, submits []string
	n := Textarea(TextareaOptions{
		OnInput:  func(v string) { inputs = append(inputs, v) },
		OnChange: func(v string) { changes = append(changes, v) },
		OnSubmit: func(v string) { submits = append(submits, v) },
	})
	r := testRenderer(NewNode(Style{}, n))
	r.Focus(n)
	n.keyHandler(key("h", "h"))
	k := key("return", "\r")
	n.keyHandler(k)
	if !k.DefaultPrevented() || n.Value() != "h\n" {
		t.Fatalf("Enter should insert a newline: %q", n.Value())
	}
	n.keyHandler(key("i", "i"))
	n.keyHandler(key("return", "\x1b\r", "meta"))
	if len(submits) != 1 || submits[0] != "h\ni" {
		t.Fatalf("Meta+Enter submit: %v", submits)
	}
	if len(inputs) != 3 || len(changes) != 1 {
		t.Fatalf("inputs %v changes %v", inputs, changes)
	}
	r.Blur()
	if len(changes) != 1 {
		t.Fatalf("blur after submit repeated OnChange: %v", changes)
	}
}

func TestTextareaPaintScrollsToCursor(t *testing.T) {
	n := Textarea(TextareaOptions{
		Value: "l0\nl1\nl2\nl3\nl4",
		Style: Style{Width: Cells(4), Height: Cells(3)},
	})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	r := testRenderer(root)
	r.Focus(n)
	b := draw(root, 4, 3)
	if got := screen(b); got != "l2  \nl3  \nl4  \n" {
		t.Fatalf("scrolled to cursor at the end:\n%s", got)
	}
	if c, _ := b.Get(2, 2); c.Attr&AttrInverse == 0 {
		t.Fatalf("cursor not drawn after l4: %+v", c)
	}
	n.SetCursorPosition(0, 0)
	b = draw(root, 4, 3)
	if got := screen(b); got != "l0  \nl1  \nl2  \n" {
		t.Fatalf("scrolled back to the top:\n%s", got)
	}
}

func TestTextareaPaintHorizontalScrollByCells(t *testing.T) {
	n := Textarea(TextareaOptions{
		Value: "ab界cd\nxy",
		Style: Style{Width: Cells(4), Height: Cells(2)},
	})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	r := testRenderer(root)
	r.Focus(n)
	n.SetCursorPosition(0, 5) // end of line 0: cell 6
	b := draw(root, 4, 2)
	// Visible cells 3..6: the trailing half of 界 is at cell 3, which cannot
	// be drawn alone, so it stays blank; then c, d, cursor.
	if c, _ := b.Get(0, 0); c.Char != ' ' {
		t.Fatalf("partial wide glyph drawn: %+v", c)
	}
	if c, _ := b.Get(1, 0); c.Char != 'c' {
		t.Fatalf("cell 1 = %+v, want c", c)
	}
	if c, _ := b.Get(3, 0); c.Attr&AttrInverse == 0 {
		t.Fatalf("cursor not at the right edge: %+v", c)
	}
	// Every line shares the horizontal offset, so "xy" is scrolled out.
	if got := strings.TrimSpace(strings.Split(screen(b), "\n")[1]); got != "" {
		t.Fatalf("second line should be scrolled out of view: %q", got)
	}
}

func TestTextareaPlaceholderAndMeasure(t *testing.T) {
	n := Textarea(TextareaOptions{Placeholder: "notes"})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	ComputeLayout(root, 20, 10)
	if r := n.Rect(); r.W != 5 || r.H != 1 {
		t.Fatalf("empty measure = %dx%d, want 5x1", r.W, r.H)
	}
	b := draw(root, 6, 1)
	if got := screen(b); got != "notes \n" {
		t.Fatalf("placeholder: %q", got)
	}
	n.SetValue("a\nlonger\nb")
	ComputeLayout(root, 20, 10)
	if r := n.Rect(); r.W != 7 || r.H != 3 { // +1 column for an end-of-line cursor
		t.Fatalf("measure = %dx%d, want 7x3", r.W, r.H)
	}
}

// FuzzTextareaKeys checks editor invariants after arbitrary key sequences:
// the cursor stays in bounds and on a grapheme boundary, no line contains a
// newline, and Value round-trips through SetValue.
func FuzzTextareaKeys(f *testing.F) {
	f.Add("ab\ncd", []byte{0, 1, 2, 3, 4, 5, 6})
	f.Add("A界B\né\n👩🏽‍💻", []byte{7, 8, 9, 10, 11, 12, 13, 14, 15})
	f.Add("", []byte{4, 4, 0, 5, 16, 16, 2, 3})
	keys := []*KeyEvent{
		key("left", "\x1b[D"), key("right", "\x1b[C"), key("up", "\x1b[A"), key("down", "\x1b[B"),
		key("return", "\r"), key("backspace", "\x7f"), key("delete", "\x1b[3~"),
		key("home", "\x1b[H"), key("end", "\x1b[F"), key("a", "\x01", "ctrl"), key("e", "\x05", "ctrl"),
		key("k", "\x0b", "ctrl"), key("u", "\x15", "ctrl"), key("w", "\x17", "ctrl"),
		key("x", "x"), key("界", "界"), key("", "́"),
	}
	f.Fuzz(func(t *testing.T, value string, ops []byte) {
		n := Textarea(TextareaOptions{Value: value})
		ops = ops[:min(len(ops), 300)] // longer lists only slow the fuzzer
		for _, op := range ops {
			k := *keys[int(op)%len(keys)]
			n.keyHandler(&k)
			s := n.textarea
			if s.line < 0 || s.line >= len(s.lines) || s.col < 0 || s.col > len(s.lines[s.line]) {
				t.Fatalf("cursor out of bounds: (%d,%d) in %q", s.line, s.col, n.Value())
			}
			if inputFloor(s.lines[s.line], s.col) != s.col {
				t.Fatalf("cursor inside a cluster: (%d,%d) in %q", s.line, s.col, n.Value())
			}
			for _, line := range s.lines {
				if strings.ContainsRune(string(line), '\n') {
					t.Fatalf("line contains newline: %q", string(line))
				}
			}
		}
		v := n.Value()
		m := Textarea(TextareaOptions{Value: v})
		if m.Value() != v {
			t.Fatalf("round trip: %q -> %q", v, m.Value())
		}
	})
}
