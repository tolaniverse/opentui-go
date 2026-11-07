package opentui

import (
	"strings"
	"testing"
)

// editorAt puts a Textarea of w×h at the top-left of an otherwise empty
// screen and paints one frame, so hit-testing and rows are ready.
func editorAt(o TextareaOptions, w, h int) (*Renderer, *Node) {
	o.Style.Width, o.Style.Height = Cells(w), Cells(h)
	n := Textarea(o)
	r := testRenderer(NewNode(Style{AlignItems: AlignStart}, n))
	r.drawFrame(NewBuffer(w+4, h+2))
	return r, n
}

func frame(r *Renderer, n *Node) *Buffer {
	rect := n.Rect()
	b := NewBuffer(rect.W+4, rect.H+2)
	r.drawFrame(b)
	return b
}

func click(r *Renderer, x, y int, mods ...string) {
	down := &MouseEvent{Type: MouseDown, Button: MouseLeft, X: x, Y: y}
	for _, m := range mods {
		if m == "shift" {
			down.Modifiers.Shift = true
		}
	}
	r.dispatchMouse(down)
	r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: x, Y: y})
}

func wheel(r *Renderer, x, y int, dir ScrollDirection) *MouseEvent {
	e := &MouseEvent{Type: MouseScroll, X: x, Y: y, Scroll: &ScrollInfo{Direction: dir, Delta: 1}}
	r.dispatchMouse(e)
	return e
}

func TestTextareaClickPlacesCursorAndFocuses(t *testing.T) {
	r, n := editorAt(TextareaOptions{Value: "hello\nworld"}, 10, 3)
	click(r, 2, 1)
	if !n.Focused() {
		t.Fatal("click did not focus the editor")
	}
	cursorAt(t, n, 1, 2)
	click(r, 8, 0) // past the end of "hello"
	cursorAt(t, n, 0, 5)
	click(r, 1, 2) // below the content
	cursorAt(t, n, 1, 5)
	if got := n.SelectedText(); got != "" {
		t.Fatalf("plain clicks selected %q", got)
	}
}

func TestTextareaClickUsesCellColumns(t *testing.T) {
	r, n := editorAt(TextareaOptions{Value: "A界B"}, 8, 1)
	click(r, 2, 0) // trailing half of 界
	cursorAt(t, n, 0, 1)
	click(r, 3, 0)
	cursorAt(t, n, 0, 2)
}

func TestTextareaClickOnWrappedRow(t *testing.T) {
	r, n := editorAt(TextareaOptions{Value: "aaaa bbbb cccc", Wrap: WrapWord}, 6, 3)
	click(r, 2, 1) // "bbbb " row
	cursorAt(t, n, 0, 7)
	click(r, 5, 0) // past the hanging space of row 0: stays on row 0
	cursorAt(t, n, 0, 4)
}

func TestTextareaDragSelectsAndTypingReplaces(t *testing.T) {
	var selected []string
	r, n := editorAt(TextareaOptions{Value: "hello\nworld", OnSelect: func(s string) { selected = append(selected, s) }}, 10, 3)
	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 1, Y: 0})
	r.dispatchMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: 3, Y: 1})
	if got := n.SelectedText(); got != "ello\nwor" {
		t.Fatalf("drag selection = %q", got)
	}
	r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: 3, Y: 1})
	if len(selected) != 1 || selected[0] != "ello\nwor" {
		t.Fatalf("OnSelect calls: %q", selected)
	}
	b := frame(r, n)
	if c, _ := b.Get(2, 0); c.BG != n.textarea.opts.SelectionBG {
		t.Fatalf("selected cell not highlighted: %+v", c)
	}
	if c, _ := b.Get(5, 0); c.BG != n.textarea.opts.SelectionBG {
		t.Fatalf("selected line break not shown after line end: %+v", c)
	}
	if c, _ := b.Get(4, 1); c.BG == n.textarea.opts.SelectionBG {
		t.Fatalf("cell after the selection highlighted: %+v", c)
	}
	n.keyHandler(key("X", "X"))
	if got := n.Value(); got != "hXld" || n.SelectedText() != "" {
		t.Fatalf("typing over selection: %q (selection %q)", got, n.SelectedText())
	}
	cursorAt(t, n, 0, 2)
}

func TestTextareaDragOutsideClampsAndScrolls(t *testing.T) {
	r, n := editorAt(TextareaOptions{Value: "l0\nl1\nl2\nl3\nl4\nl5"}, 4, 3)
	n.SetCursorPosition(0, 0)
	frame(r, n)
	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 0, Y: 0})
	r.dispatchMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: 1, Y: 3}) // one row below the box
	cursorAt(t, n, 3, 1)
	frame(r, n) // the cursor's row scrolls into view
	if top := n.textarea.topLine; top != 1 {
		t.Fatalf("drag below did not scroll: top line %d", top)
	}
	r.dispatchMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: 0, Y: -5}) // far above
	cursorAt(t, n, 0, 0)
	r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: 0, Y: -5})
	if got := n.SelectedText(); got != "" {
		t.Fatalf("drag back to the anchor left a selection: %q", got)
	}
}

func TestTextareaShiftSelectionAndCollapse(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "hello\nworld"})
	n.SetCursorPosition(0, 0)
	n.keyHandler(key("right", "\x1b[1;2C", "shift"))
	n.keyHandler(key("right", "\x1b[1;2C", "shift"))
	if got := n.SelectedText(); got != "he" {
		t.Fatalf("shift+right x2 = %q", got)
	}
	n.keyHandler(key("down", "\x1b[1;2B", "shift"))
	if got := n.SelectedText(); got != "hello\nwo" {
		t.Fatalf("shift+down = %q", got)
	}
	n.keyHandler(key("left", "\x1b[D")) // collapses to the start
	cursorAt(t, n, 0, 0)
	if n.SelectedText() != "" {
		t.Fatal("left did not collapse the selection")
	}
	n.keyHandler(key("e", "\x05", "ctrl", "shift")) // select to line end
	if got := n.SelectedText(); got != "hello" {
		t.Fatalf("shift+ctrl+e = %q", got)
	}
	n.keyHandler(key("right", "\x1b[C")) // collapses to the end
	cursorAt(t, n, 0, 5)
	n.keyHandler(key("home", "\x1b[1;2H", "shift"))
	n.keyHandler(key("backspace", "\x7f"))
	if got := n.Value(); got != "\nworld" {
		t.Fatalf("backspace over selection: %q", got)
	}
	cursorAt(t, n, 0, 0)
}

func TestTextareaSelectAllAndDeleteKinds(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "one\ntwo"})
	k := key("a", "a")
	k.Super = true
	n.keyHandler(k)
	if got := n.SelectedText(); got != "one\ntwo" {
		t.Fatalf("Super+A = %q", got)
	}
	n.keyHandler(key("k", "\x0b", "ctrl")) // any delete key removes the selection
	if got := n.Value(); got != "" {
		t.Fatalf("Ctrl+K over selection: %q", got)
	}
	n.SetValue("abc")
	n.SelectAll()
	n.InsertText("x\ny") // paste replaces the selection
	if got := n.Value(); got != "x\ny" {
		t.Fatalf("paste over selection: %q", got)
	}
}

func TestTextareaDoubleClickSelectsWord(t *testing.T) {
	r, n := editorAt(TextareaOptions{Value: "foo bar_baz qux"}, 20, 1)
	click(r, 6, 0)
	click(r, 6, 0)
	if got := n.SelectedText(); got != "bar_baz" {
		t.Fatalf("double-click = %q", got)
	}
	cursorAt(t, n, 0, 11)
	click(r, 2, 0, "shift") // shift-click moves the cursor; the anchor (word start) stays
	if got := n.SelectedText(); got != "o " {
		t.Fatalf("shift-click after double-click = %q", got)
	}
}

func TestTextareaWheelScrollsWithoutMovingCursor(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, "l"+string(rune('0'+i)))
	}
	r, n := editorAt(TextareaOptions{Value: strings.Join(lines, "\n")}, 4, 3)
	b := frame(r, n)
	if got := screen(b)[:3]; got != "l7 " {
		t.Fatalf("initial top row %q", got)
	}
	if e := wheel(r, 1, 1, ScrollUp); !e.PropagationStopped() {
		t.Fatal("a wheel that scrolled must stop propagation")
	}
	b = frame(r, n)
	if got := screen(b)[:3]; got != "l4 " {
		t.Fatalf("after wheel up, top row %q", got)
	}
	cursorAt(t, n, 9, 2)
	for i := 0; i < 5; i++ {
		wheel(r, 1, 1, ScrollUp)
	}
	frame(r, n)
	if e := wheel(r, 1, 1, ScrollUp); e.PropagationStopped() {
		t.Fatal("a wheel at the top must bubble to a parent")
	}
	n.keyHandler(key("x", "x")) // typing brings the cursor back into view
	b = frame(r, n)
	if got := screen(b)[:3]; got != "l7 " {
		t.Fatalf("after typing, top row %q", got)
	}
}

// FuzzTextareaMouse mixes keys (with and without Shift) and mouse actions
// on a wrapping editor; the cursor and anchor must stay valid and the
// selection must be text that exists in the value.
func FuzzTextareaMouse(f *testing.F) {
	f.Add("hello world\nfoo 界 bar\n\né", []byte{0, 20, 40, 60, 80, 100, 120, 140, 160, 180, 200, 220, 240}, uint8(7), uint8(3), true)
	f.Add("", []byte{1, 2, 3, 250, 251, 252}, uint8(1), uint8(1), false)
	keys := []*KeyEvent{
		key("left", "\x1b[D"), key("right", "\x1b[C"), key("up", "\x1b[A"), key("down", "\x1b[B"),
		key("left", "\x1b[1;2D", "shift"), key("right", "\x1b[1;2C", "shift"),
		key("up", "\x1b[1;2A", "shift"), key("down", "\x1b[1;2B", "shift"),
		key("home", "\x1b[1;2H", "shift"), key("end", "\x1b[1;2F", "shift"),
		key("return", "\r"), key("backspace", "\x7f"), key("delete", "\x1b[3~"),
		key("x", "x"), key("界", "界"), key("w", "\x17", "ctrl"),
	}
	f.Fuzz(func(t *testing.T, value string, ops []byte, w, h uint8, wrap bool) {
		width, height := int(w%10)+1, int(h%5)+1
		o := TextareaOptions{Value: value}
		if wrap {
			o.Wrap = WrapWord
		}
		r, n := editorAt(o, width, height)
		ops = ops[:min(len(ops), 200)]
		for _, op := range ops {
			x, y := int(op%13)-1, int(op/13%9)-2
			switch op % 6 {
			case 0:
				r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: x, Y: y})
			case 1:
				r.dispatchMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: x, Y: y})
			case 2:
				r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: x, Y: y})
			case 3:
				dir := ScrollUp
				if op&8 != 0 {
					dir = ScrollDown
				}
				wheel(r, x, y, dir)
			default:
				k := *keys[int(op/6)%len(keys)]
				n.keyHandler(&k)
			}
			frame(r, n)
			s := n.textarea
			valid := func(line, col int) bool {
				return line >= 0 && line < len(s.lines) && col >= 0 && col <= len(s.lines[line]) &&
					inputFloor(s.lines[line], col) == col
			}
			if !valid(s.line, s.col) {
				t.Fatalf("bad cursor (%d,%d) in %q", s.line, s.col, n.Value())
			}
			if s.hasAnchor && !valid(s.anchorLine, s.anchorCol) {
				t.Fatalf("bad anchor (%d,%d) in %q", s.anchorLine, s.anchorCol, n.Value())
			}
			if sel := n.SelectedText(); !strings.Contains(n.Value(), sel) {
				t.Fatalf("selection %q not in value %q", sel, n.Value())
			}
		}
	})
}
