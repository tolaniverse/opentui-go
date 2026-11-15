// canvas shows custom drawing inside the layout: a Canvas node sits in a
// bordered box next to ordinary text, and paints a shaded sphere with
// Buffer.DrawImage. Each cell shows two pixels (a half block), so the
// canvas is twice as tall in pixels as in cells and pixels are roughly
// square.
//
// Left/Right move the light, Q or Ctrl+C quits.
//
//	go run ./examples/canvas
package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"

	tui "github.com/tolaniverse/opentui-go"
)

// sphere renders a lit ball into a w×h image. angle swings the light
// around the vertical axis. Pixels off the ball stay transparent, so the
// box's background shows through.
func sphere(w, h int, angle float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	radius := 0.45 * float64(min(w, h))
	lx, ly, lz := math.Sin(angle)*0.8, -0.5, math.Cos(angle)*0.8
	ll := math.Sqrt(lx*lx + ly*ly + lz*lz)
	for y := range h {
		for x := range w {
			// Position on the unit sphere under this pixel's centre.
			nx := (float64(x) + 0.5 - float64(w)/2) / radius
			ny := (float64(y) + 0.5 - float64(h)/2) / radius
			d := nx*nx + ny*ny
			if d > 1 {
				continue
			}
			nz := math.Sqrt(1 - d)
			diffuse := math.Max(0, (nx*lx+ny*ly+nz*lz)/ll)
			shine := math.Pow(diffuse, 24)
			level := func(base float64) uint8 {
				return uint8(255 * math.Min(1, base*(0.08+0.92*diffuse)+0.6*shine))
			}
			img.SetNRGBA(x, y, color.NRGBA{level(1.0), level(0.30), level(0.14), 255})
		}
	}
	return img
}

func main() {
	r, err := tui.New(tui.Options{Background: tui.MustHex("#1e1e2e"), Title: "opentui-go · canvas"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "canvas:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Paint and the key handler both run on the Run goroutine, so this
	// state needs no lock.
	var (
		angle  = -0.7
		cached *image.NRGBA
		drawn  float64
	)
	canvas := tui.Canvas(tui.CanvasOptions{
		Style: tui.Style{FlexGrow: 1},
		Paint: func(b *tui.Buffer, area tui.Rect) {
			// Render at exactly the canvas's pixel size, and only when the
			// size or the light has changed.
			w, h := area.W, area.H*2
			if cached == nil || cached.Rect.Dx() != w || cached.Rect.Dy() != h || drawn != angle {
				cached, drawn = sphere(w, h, angle), angle
			}
			b.DrawImage(area, cached)
		},
	})

	root := r.Root()
	root.Style = tui.Style{Padding: tui.XY(2, 1), Gap: 1}
	root.Add(
		tui.Box(tui.BoxOptions{
			Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: tui.MustHex("#89b4fa"),
			Title: " canvas ", Style: tui.Style{FlexGrow: 1},
		}, canvas),
		tui.Text("←/→ move the light · Q quits", tui.TextOptions{FG: tui.MustHex("#a6adc8")}),
	)

	r.SetKeyHandler(func(k *tui.KeyEvent) {
		if k.Type == tui.KeyRelease {
			return
		}
		switch k.Name {
		case "left":
			angle -= 0.2
		case "right":
			angle += 0.2
		case "q":
			cancel()
			return
		default:
			return
		}
		r.RequestRender() // what Paint draws has changed
	})

	if err := r.Run(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "canvas:", err)
		os.Exit(1)
	}
}
