package opentui

import (
	"image"
	"image/color"
	"testing"
)

func TestCanvasPaintsClippedToItsRect(t *testing.T) {
	var got Rect
	c := Canvas(CanvasOptions{Style: Style{Width: Cells(3), Height: Cells(2)}, Paint: func(b *Buffer, area Rect) {
		got = area
		for y := -1; y <= area.H; y++ {
			for x := -1; x <= area.W; x++ {
				b.Set(area.X+x, area.Y+y, Cell{Char: '#', FG: White})
			}
		}
	}})
	root := NewNode(Style{Padding: All(1)}, c)
	b := NewBuffer(6, 5)
	Draw(root, b)
	if got != (Rect{X: 1, Y: 1, W: 3, H: 2}) {
		t.Fatalf("area = %+v", got)
	}
	for y := 0; y < 5; y++ {
		for x := 0; x < 6; x++ {
			cell, _ := b.Get(x, y)
			inside := x >= 1 && x < 4 && y >= 1 && y < 3
			if (cell.Char == '#') != inside {
				t.Fatalf("cell (%d,%d) = %q, inside = %v", x, y, cell.Char, inside)
			}
		}
	}
}

func TestCanvasNilPaintAndSetPaint(t *testing.T) {
	c := Canvas(CanvasOptions{Style: Style{Width: Cells(1), Height: Cells(1)}})
	b := NewBuffer(1, 1)
	Draw(c, b) // nil Paint draws nothing and must not panic
	c.SetPaint(func(b *Buffer, a Rect) { b.Set(a.X, a.Y, Cell{Char: 'x'}) })
	Draw(c, b)
	if cell, _ := b.Get(0, 0); cell.Char != 'x' {
		t.Fatalf("cell = %q", cell.Char)
	}
}

func TestDrawImageUsesHalfBlocks(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	img.Set(0, 1, color.NRGBA{0, 0, 255, 255})
	img.Set(1, 0, color.NRGBA{0, 255, 0, 255})
	// (1,1) transparent: keeps the cell background; row 2 has no partner.
	img.Set(0, 2, color.NRGBA{9, 9, 9, 255})
	b := NewBuffer(3, 3)
	b.Clear(RGB(1, 2, 3))
	b.DrawImage(Rect{W: 3, H: 3}, img)
	want := []struct {
		x, y   int
		fg, bg RGBA
	}{
		{0, 0, RGB(255, 0, 0), RGB(0, 0, 255)},
		{1, 0, RGB(0, 255, 0), RGB(1, 2, 3)},
		{0, 1, RGB(9, 9, 9), RGB(1, 2, 3)},
	}
	for _, w := range want {
		c, _ := b.Get(w.x, w.y)
		if c.Char != HalfBlock || c.FG != w.fg || c.BG != w.bg {
			t.Fatalf("(%d,%d) = %q %v/%v, want %v/%v", w.x, w.y, c.Char, c.FG, c.BG, w.fg, w.bg)
		}
	}
	if c, _ := b.Get(2, 0); c.Char == HalfBlock {
		t.Fatal("drew past the image")
	}
}
