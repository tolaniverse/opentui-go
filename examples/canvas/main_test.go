package main

import (
	"testing"

	tui "github.com/tolaniverse/opentui-go"
)

func TestSphereDrawsInsideTheCanvasOnly(t *testing.T) {
	bg := tui.RGB(1, 2, 3)
	canvas := tui.Canvas(tui.CanvasOptions{
		Style: tui.Style{Width: tui.Cells(20), Height: tui.Cells(10)},
		Paint: func(b *tui.Buffer, area tui.Rect) { b.DrawImage(area, sphere(area.W, area.H*2, 0)) },
	})
	b := tui.NewBuffer(24, 12)
	b.Clear(bg)
	tui.Draw(tui.NewNode(tui.Style{Padding: tui.All(1)}, canvas), b)

	centre, _ := b.Get(11, 6)
	if centre.Char != tui.HalfBlock || centre.FG.R <= centre.FG.B {
		t.Fatalf("centre = %q %v, want a lit orange half block", centre.Char, centre.FG)
	}
	// The canvas corner is off the ball (transparent) and the padding is
	// outside the canvas: both keep the background.
	for _, p := range [][2]int{{1, 1}, {0, 0}, {23, 11}} {
		if c, _ := b.Get(p[0], p[1]); c.Char == tui.HalfBlock || c.BG != bg {
			t.Fatalf("cell %v = %q %v, want untouched background", p, c.Char, c.BG)
		}
	}
}
