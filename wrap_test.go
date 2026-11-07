package opentui

import (
	"reflect"
	"strings"
	"testing"
)

// rowTexts wraps line and returns each visual row with trailing spaces
// trimmed (they hang past the edge and are not painted).
func rowTexts(line string, width int, mode WrapMode) []string {
	runes := []rune(line)
	var out []string
	for _, row := range wrapLine(runes, width, mode) {
		out = append(out, strings.TrimRight(string(runes[row.start:row.end]), " "))
	}
	return out
}

func TestWrapLineModes(t *testing.T) {
	for _, tc := range []struct {
		line  string
		width int
		mode  WrapMode
		want  []string
	}{
		{"the quick brown fox", 10, WrapWord, []string{"the quick", "brown fox"}},
		{"the quick brown fox", 9, WrapWord, []string{"the quick", "brown fox"}},
		{"abcdefghij kl", 4, WrapWord, []string{"abcd", "efgh", "ij", "kl"}}, // long word: char fallback
		{"hello world", 4, WrapChar, []string{"hell", "o wo", "rld"}},
		{"界界界", 5, WrapWord, []string{"界界", "界"}}, // never half a wide glyph
		{"a ééé", 3, WrapChar, []string{"a é", "éé"}},
		{"short", 10, WrapWord, []string{"short"}},
		{"", 5, WrapWord, []string{""}},
		{"no wrap at all", 4, WrapNone, []string{"no wrap at all"}},
		{"ab", 0, WrapWord, []string{"ab"}}, // no width yet: one row
	} {
		if got := rowTexts(tc.line, tc.width, tc.mode); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("wrap(%q, %d, %v) = %q, want %q", tc.line, tc.width, tc.mode, got, tc.want)
		}
	}
}

func TestWrapRowsCoverLineExactly(t *testing.T) {
	line := []rune("lorem ipsum 界 dolor sit-amet é 👩🏽‍💻 end")
	for width := 1; width <= 12; width++ {
		for _, mode := range []WrapMode{WrapChar, WrapWord} {
			rows := wrapLine(line, width, mode)
			pos := 0
			for _, row := range rows {
				if row.start != pos || row.end <= row.start {
					t.Fatalf("width %d mode %v: rows %+v do not tile the line", width, mode, rows)
				}
				if inputFloor(line, row.start) != row.start {
					t.Fatalf("width %d: row starts inside a cluster: %+v", width, row)
				}
				pos = row.end
			}
			if pos != len(line) {
				t.Fatalf("width %d mode %v: rows end at %d of %d", width, mode, pos, len(line))
			}
		}
	}
}

func TestTextWrapMeasureAndPaint(t *testing.T) {
	n := Text("the quick brown fox\nok", TextOptions{Wrap: WrapWord})
	root := NewNode(Style{Width: Cells(10), AlignItems: AlignStart}, n)
	b := draw(root, 12, 4)
	if r := n.Rect(); r.W != 9 || r.H != 3 {
		t.Fatalf("wrapped size = %dx%d, want 9x3", r.W, r.H)
	}
	if got := screen(b); got != "the quick   \nbrown fox   \nok          \n            \n" {
		t.Fatalf("wrapped paint:\n%s", got)
	}
	// Without a width limit, wrapping text reports its natural size.
	free := Text("the quick brown fox", TextOptions{Wrap: WrapWord})
	ComputeLayout(NewNode(Style{AlignItems: AlignStart}, free), 40, 4)
	if r := free.Rect(); r.W != 19 || r.H != 1 {
		t.Fatalf("unconstrained size = %dx%d, want 19x1", r.W, r.H)
	}
}

func TestTextWrapReflowsOnResize(t *testing.T) {
	n := Text("aa bb cc dd", TextOptions{Wrap: WrapWord})
	root := NewNode(Style{}, n) // stretched to the screen width
	draw(root, 6, 4)
	if h := n.Rect().H; h != 2 {
		t.Fatalf("height at width 6 = %d, want 2", h)
	}
	draw(root, 3, 4)
	if h := n.Rect().H; h != 4 {
		t.Fatalf("height at width 3 = %d, want 4", h)
	}
}

func TestSelectionAcrossSoftWrapHasNoNewline(t *testing.T) {
	n := Text("the quick brown fox\nok", TextOptions{Wrap: WrapWord})
	r := testRenderer(NewNode(Style{Width: Cells(10), AlignItems: AlignStart}, n))
	r.drawFrame(NewBuffer(12, 4))
	mouse(r, MouseDown, 4, 0) // "quick"
	mouse(r, MouseDrag, 4, 1) // "brown"
	if got := r.SelectedText(); got != "quick brown" {
		t.Fatalf("soft-wrapped selection = %q, want %q", got, "quick brown")
	}
	mouse(r, MouseDrag, 1, 2) // into the next hard line
	if got := r.SelectedText(); got != "quick brown fox\nok" {
		t.Fatalf("selection across a hard newline = %q", got)
	}
	b := NewBuffer(12, 4)
	r.drawFrame(b)
	if c, _ := b.Get(0, 1); c.BG != n.text.opts.SelectionBG {
		t.Fatalf("wrapped row not highlighted: %+v", c)
	}
}

func textareaRows(b *Buffer, rows int) []string {
	return strings.Split(screen(b), "\n")[:rows]
}

func TestTextareaWrapPaintAndVisualRows(t *testing.T) {
	n := Textarea(TextareaOptions{
		Value: "aaaa bbbb cccc", Wrap: WrapWord,
		Style: Style{Width: Cells(6), Height: Cells(3)},
	})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	r := testRenderer(root)
	r.Focus(n)
	b := draw(root, 6, 3)
	if got := textareaRows(b, 3); !reflect.DeepEqual(got, []string{"aaaa  ", "bbbb  ", "cccc  "}) {
		t.Fatalf("wrapped rows: %q", got)
	}
	if c, _ := b.Get(4, 2); c.Attr&AttrInverse == 0 {
		t.Fatalf("cursor not after cccc on row 2: %+v", c)
	}
	n.keyHandler(key("up", "\x1b[A"))
	cursorAt(t, n, 0, 9) // visual row 1, cell 4
	n.keyHandler(key("up", "\x1b[A"))
	cursorAt(t, n, 0, 4)
	n.keyHandler(key("up", "\x1b[A"))
	cursorAt(t, n, 0, 0) // top visual row: go to its start
	n.keyHandler(key("down", "\x1b[B"))
	n.keyHandler(key("down", "\x1b[B"))
	cursorAt(t, n, 0, 10) // goal cell 0 on row 2
	n.keyHandler(key("down", "\x1b[B"))
	cursorAt(t, n, 0, 14) // bottom visual row: go to its end

	// A cursor exactly at a soft break is shown at the start of the next row.
	n.SetCursorPosition(0, 5)
	b = draw(root, 6, 3)
	if c, _ := b.Get(0, 1); c.Char != 'b' || c.Attr&AttrInverse == 0 {
		t.Fatalf("cursor at soft break not at row 1 start: %+v", c)
	}
}

func TestTextareaWrapUpDownLandsOnTheRightRow(t *testing.T) {
	// Row 0 is "aaaa " (offsets 0-4); its end offset 5 belongs to row 1.
	// Moving up from row 1 with a large goal must stay on row 0.
	n := Textarea(TextareaOptions{Value: "aaaa bbbbb", Wrap: WrapWord, Style: Style{Width: Cells(6), Height: Cells(2)}})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	r := testRenderer(root)
	r.Focus(n)
	draw(root, 6, 2)
	n.keyHandler(key("up", "\x1b[A")) // from the end of "bbbbb", goal cell 5
	if l, c := n.CursorPosition(); l != 0 || c != 4 {
		t.Fatalf("up from row 1 = (%d,%d), want (0,4) on row 0", l, c)
	}
}

func TestTextareaWrapScrollsByVisualRows(t *testing.T) {
	n := Textarea(TextareaOptions{
		Value: strings.Repeat("xx yy\n", 4) + "zz qq", Wrap: WrapWord,
		Style: Style{Width: Cells(3), Height: Cells(3)},
	})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	r := testRenderer(root)
	r.Focus(n)
	b := draw(root, 3, 3)
	if got := textareaRows(b, 3); !reflect.DeepEqual(got, []string{"yy ", "zz ", "qq "}) {
		t.Fatalf("scrolled wrapped rows: %q", got)
	}
	n.SetCursorPosition(0, 0)
	b = draw(root, 3, 3)
	if got := textareaRows(b, 3); !reflect.DeepEqual(got, []string{"xx ", "yy ", "xx "}) {
		t.Fatalf("scrolled back to top: %q", got)
	}
	// Deleting content pulls the view back so no blank rows are left below.
	n.SetValue("on") // wraps at width-1 = 2, leaving the last column for the cursor
	b = draw(root, 3, 3)
	if got := textareaRows(b, 1); got[0] != "on " {
		t.Fatalf("after shrinking, first row = %q", got[0])
	}
}

func TestTextareaWrapMeasuresHeight(t *testing.T) {
	n := Textarea(TextareaOptions{Value: "aa bb cc", Wrap: WrapWord, Style: Style{Width: Cells(4)}})
	ComputeLayout(NewNode(Style{AlignItems: AlignStart}, n), 20, 10)
	if h := n.Rect().H; h != 3 {
		t.Fatalf("wrapped textarea height = %d, want 3", h)
	}
}

// FuzzTextareaWrapPaint edits and paints a wrapping editor at random sizes;
// the cursor must stay valid and painting must never panic.
func FuzzTextareaWrapPaint(f *testing.F) {
	f.Add("aaaa bbbb cccc\n界界界 é", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}, uint8(5), uint8(3))
	f.Add("", []byte{14, 14, 14, 4, 2, 2, 3}, uint8(1), uint8(1))
	keys := []*KeyEvent{
		key("left", "\x1b[D"), key("right", "\x1b[C"), key("up", "\x1b[A"), key("down", "\x1b[B"),
		key("return", "\r"), key("backspace", "\x7f"), key("delete", "\x1b[3~"),
		key("home", "\x1b[H"), key("end", "\x1b[F"), key("k", "\x0b", "ctrl"),
		key("w", "\x17", "ctrl"), key("x", "x"), key("界", "界"), key(" ", " "), key("y", "y"),
	}
	f.Fuzz(func(t *testing.T, value string, ops []byte, w, h uint8) {
		width, height := int(w%12)+1, int(h%6)+1
		// Each op repaints; long op lists only slow the fuzzer past its hang
		// threshold without reaching new states.
		ops = ops[:min(len(ops), 300)]
		for _, mode := range []WrapMode{WrapChar, WrapWord} {
			n := Textarea(TextareaOptions{Value: value, Wrap: mode, Style: Style{Width: Cells(width), Height: Cells(height)}})
			root := NewNode(Style{AlignItems: AlignStart}, n)
			r := testRenderer(root)
			r.Focus(n)
			buf := NewBuffer(width, height)
			Draw(root, buf)
			for _, op := range ops {
				k := *keys[int(op)%len(keys)]
				n.keyHandler(&k)
				buf.Clear(Transparent) // as the renderer does each frame
				Draw(root, buf)
				s := n.textarea
				if s.line < 0 || s.line >= len(s.lines) || s.col < 0 || s.col > len(s.lines[s.line]) ||
					inputFloor(s.lines[s.line], s.col) != s.col {
					t.Fatalf("bad cursor (%d,%d) in %q", s.line, s.col, n.Value())
				}
				if s.topLine < 0 || s.topLine >= len(s.lines) {
					t.Fatalf("top line %d out of range", s.topLine)
				}
				visible := false
				for y := 0; y < height && !visible; y++ {
					for x := 0; x < width; x++ {
						if c, _ := buf.Get(x, y); c.Attr&AttrInverse != 0 {
							visible = true
							break
						}
					}
				}
				if !visible {
					t.Fatalf("cursor (%d,%d) not visible at %dx%d, mode %v, value %q", s.line, s.col, width, height, mode, n.Value())
				}
			}
		}
	})
}
