package opentui

import (
	"bytes"
	"strings"
	"testing"
)

func TestUnicodeCellsAndMeasurement(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
	}{
		{"e\u0301", 1}, {"界", 2}, {"👩🏽‍💻", 2}, {"A界B", 4}, {"⸺X", 4},
	} {
		n := Text(tc.text, TextOptions{})
		root := NewNode(Style{AlignItems: AlignStart}, n)
		ComputeLayout(root, 10, 2)
		if got := n.Rect().W; got != tc.width {
			t.Errorf("%q measured %d cells, want %d", tc.text, got, tc.width)
		}
	}
	b := NewBuffer(6, 1)
	b.DrawText("A界B", 0, 0, White, Transparent, AttrNone)
	if c, _ := b.Get(3, 0); c.Char != 'B' {
		t.Fatalf("B after a wide character landed at cell 3 as %q", c.Char)
	}
}

func TestUnicodeFrameEmitsClusterOnce(t *testing.T) {
	current, next := newPair(5, 1)
	next.DrawText("👩🏽‍💻!", 0, 0, White, Transparent, AttrNone)
	var out bytes.Buffer
	encodeFrame(&out, current, next, FrameOptions{})
	if got := out.String(); !strings.Contains(got, "👩🏽‍💻!") {
		t.Fatalf("cluster not emitted as one terminal glyph: %q", got)
	}
	if strings.Count(out.String(), "👩🏽‍💻") != 1 {
		t.Fatalf("cluster emitted more than once: %q", out.String())
	}
}

func TestWideGlyphClipsAtomically(t *testing.T) {
	b := NewBuffer(2, 1)
	b.DrawText("A界", 0, 0, White, Transparent, AttrNone)
	if c, _ := b.Get(1, 0); c.Char == '界' {
		t.Fatalf("half a wide glyph drawn at right edge: %+v", c)
	}
	b = NewBuffer(3, 1)
	b.DrawText("界X", -1, 0, White, Transparent, AttrNone)
	if c, _ := b.Get(1, 0); c.Char != 'X' {
		t.Fatalf("partial left-clipped glyph corrupted X at its screen position: %+v", c)
	}
}

func TestOverwritingWideGlyphClearsBothCells(t *testing.T) {
	b := NewBuffer(3, 1)
	b.DrawText("界", 0, 0, White, Transparent, AttrNone)
	b.DrawText("X", 1, 0, White, Transparent, AttrNone)
	if c, _ := b.Get(0, 0); c.Char != ' ' {
		t.Fatalf("leading half remained after overwriting tail: %+v", c)
	}
	if c, _ := b.Get(1, 0); c.Char != 'X' {
		t.Fatalf("overwritten tail is not X: %+v", c)
	}
}

func TestUnicodeSelectionUsesCellPositions(t *testing.T) {
	n := Text("A界B", TextOptions{})
	r := testRenderer(NewNode(Style{}, n))
	r.drawFrame(NewBuffer(8, 2))
	mouse(r, MouseDown, 1, 0) // wide glyph occupies columns 1 and 2
	mouse(r, MouseDrag, 3, 0)
	if got := r.SelectedText(); got != "界B" {
		t.Fatalf("selection across wide glyph = %q, want 界B", got)
	}
	r.drawFrame(NewBuffer(8, 2))
	if got := r.SelectedText(); got != "界B" {
		t.Fatalf("selection after repaint = %q", got)
	}
	mouse(r, MouseUp, 3, 0)
	r.lastClickNode = nil     // new independent selection, not a double-click
	mouse(r, MouseDown, 2, 0) // trailing half must select the same glyph
	mouse(r, MouseDrag, 3, 0)
	if got := r.SelectedText(); got != "界B" {
		t.Fatalf("selection from trailing half = %q, want 界B", got)
	}
}

func TestWideGlyphSurvivesTranslucentOverlay(t *testing.T) {
	b := NewBuffer(3, 1)
	b.DrawText("界", 0, 0, White, RGB(10, 20, 30), AttrNone)
	b.FillRect(0, 0, 2, 1, RGBAOf(0, 0, 0, 128))
	lead, _ := b.Get(0, 0)
	tail, _ := b.Get(1, 0)
	if lead.Grapheme() != "界" || !tail.Continuation {
		t.Fatalf("translucent overlay split wide glyph: lead=%+v tail=%+v", lead, tail)
	}
}

func TestWideToNarrowDiffClearsTrailingCell(t *testing.T) {
	current, next := newPair(3, 1)
	var out bytes.Buffer
	next.DrawText("界", 0, 0, White, Transparent, AttrNone)
	encodeFrame(&out, current, next, FrameOptions{})
	out.Reset()
	next.DrawText("X", 0, 0, White, Transparent, AttrNone)
	encodeFrame(&out, current, next, FrameOptions{})
	if a, _ := current.Get(0, 0); a.Char != 'X' {
		t.Fatalf("leading cell not replaced: %+v", a)
	}
	if b, _ := current.Get(1, 0); b.Char != ' ' || b.Continuation {
		t.Fatalf("old trailing cell not cleared: %+v", b)
	}
	if !strings.Contains(out.String(), "X ") {
		t.Fatalf("terminal not sent a blank for the trailing cell: %q", out.String())
	}
}

func TestThreeCellGlyphOverwriteClearsAllCells(t *testing.T) {
	b := NewBuffer(5, 1)
	b.DrawText("⸺X", 0, 0, White, Transparent, AttrNone)
	if c, _ := b.Get(3, 0); c.Char != 'X' {
		t.Fatalf("X after three-cell glyph at wrong column: %+v", c)
	}
	b.DrawText("Y", 2, 0, White, Transparent, AttrNone)
	for _, x := range []int{0, 1} {
		if c, _ := b.Get(x, 0); c.Char != ' ' || c.Continuation {
			t.Errorf("old glyph cell %d not cleared: %+v", x, c)
		}
	}
}
