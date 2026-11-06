package opentui

import "testing"

func TestDrawTextClips(t *testing.T) {
	b := NewBuffer(3, 1)
	b.DrawText("abcdef", 1, 0, White, Transparent, AttrNone)
	want := []rune{' ', 'a', 'b'}
	for x, r := range want {
		if c, _ := b.Get(x, 0); c.Char != r {
			t.Errorf("x=%d: %q; want %q", x, c.Char, r)
		}
	}
	b.DrawText("zz", 0, 5, White, Transparent, AttrNone) // off-screen: no panic
	b.DrawText("zz", -1, 0, White, Transparent, AttrNone)
	if c, _ := b.Get(0, 0); c.Char != 'z' {
		t.Errorf("negative x should clip, got %q", c.Char)
	}
}

func TestDrawTextReplacesControlChars(t *testing.T) {
	b := NewBuffer(3, 1)
	b.DrawText("a\x1bb", 0, 0, White, Transparent, AttrNone)
	if c, _ := b.Get(1, 0); c.Char != ' ' {
		t.Errorf("ESC must not reach a cell, got %q", c.Char)
	}
}

func TestTranslucentFillKeepsText(t *testing.T) {
	b := NewBuffer(1, 1)
	b.DrawText("A", 0, 0, RGB(255, 255, 255), RGB(0, 0, 0), AttrBold)
	b.FillRect(0, 0, 1, 1, RGBAOf(0, 0, 0, 128)) // dim overlay
	c, _ := b.Get(0, 0)
	if c.Char != 'A' || c.Attr != AttrBold {
		t.Fatalf("overlay erased text: %+v", c)
	}
	if c.FG != RGB(127, 127, 127) {
		t.Errorf("text should be dimmed, fg = %v", c.FG)
	}
}

func TestOpaqueFillErases(t *testing.T) {
	b := NewBuffer(1, 1)
	b.DrawText("A", 0, 0, White, Transparent, AttrNone)
	b.FillRect(0, 0, 5, 5, RGB(10, 20, 30))
	if c, _ := b.Get(0, 0); c.Char != ' ' || c.BG != RGB(10, 20, 30) {
		t.Errorf("got %+v", c)
	}
}

func TestResize(t *testing.T) {
	b := NewBuffer(2, 2)
	b.Resize(5, 3)
	if b.Width() != 5 || b.Height() != 3 || len(b.cells) != 15 {
		t.Fatalf("bad size %dx%d len %d", b.Width(), b.Height(), len(b.cells))
	}
	if _, ok := b.Get(4, 2); !ok {
		t.Error("corner should be in bounds")
	}
	if _, ok := b.Get(5, 0); ok {
		t.Error("x=5 should be out of bounds")
	}
}
