package opentui

import (
	"image"
	"image/color"
)

// CanvasOptions configures a Canvas: a laid-out leaf that the application
// paints itself, cell by cell.
type CanvasOptions struct {
	ID     string
	Style  Style
	ZIndex int
	// Paint draws the canvas each frame, on the Run goroutine. area is the
	// canvas's layout rect; drawing is clipped to it (and to any clipping
	// ancestor such as a scroll box), so Paint may ignore the edges.
	Paint func(b *Buffer, area Rect)
}

// CanvasNode is the typed handle of a Canvas.
type CanvasNode struct{ *Node }

// Canvas returns a node whose content the application paints. Unlike the
// draw callback of Renderer.Run, it takes part in layout, z-order and
// clipping like any other node. Call Renderer.RequestRender (or change it
// inside Renderer.Update) when what Paint would draw has changed.
func Canvas(o CanvasOptions) *CanvasNode {
	n := NewNode(o.Style)
	n.ID, n.ZIndex = o.ID, o.ZIndex
	c := &CanvasNode{n}
	c.SetPaint(o.Paint)
	return c
}

// SetPaint replaces the paint function; nil paints nothing.
func (c *CanvasNode) SetPaint(fn func(b *Buffer, area Rect)) {
	if fn == nil {
		c.Node.paint = nil
		return
	}
	c.Node.paint = func(b *Buffer, n *Node) {
		if n.rect.Empty() {
			return
		}
		b.PushClip(n.rect)
		defer b.PopClip()
		fn(b, n.rect)
	}
}

// AsNode returns the underlying node, or nil for a nil handle.
func (c *CanvasNode) AsNode() *Node {
	if c == nil {
		return nil
	}
	return c.Node
}

// HalfBlock is the glyph DrawImage uses: its foreground is the upper pixel
// of a cell and its background the lower one.
const HalfBlock = '▀'

// DrawImage draws img with half blocks, so each cell shows two vertically
// stacked pixels: a W×H cell area holds a W×2H pixel image. The image's
// top-left pixel lands on area's top-left cell; pixels outside area (or the
// current clip) are skipped, and the image is not scaled. Fully transparent
// pixels keep the cell's existing background; other pixels are drawn
// opaque. Render images at (area.W, 2*area.H) for an exact fit.
func (b *Buffer) DrawImage(area Rect, img image.Image) {
	bounds := img.Bounds()
	pixel := func(x, y int) (RGBA, bool) {
		px, py := bounds.Min.X+x, bounds.Min.Y+y
		if px >= bounds.Max.X || py >= bounds.Max.Y {
			return RGBA{}, false
		}
		c := color.NRGBAModel.Convert(img.At(px, py)).(color.NRGBA)
		if c.A == 0 {
			return RGBA{}, false
		}
		return RGB(c.R, c.G, c.B), true
	}
	for cy := 0; cy < area.H; cy++ {
		for cx := 0; cx < area.W; cx++ {
			x, y := area.X+cx, area.Y+cy
			if !b.inBounds(x, y) {
				continue
			}
			top, okTop := pixel(cx, 2*cy)
			bottom, okBottom := pixel(cx, 2*cy+1)
			if !okTop && !okBottom {
				continue
			}
			old, _ := b.Get(x, y)
			if !okTop {
				top = old.BG
			}
			if !okBottom {
				bottom = old.BG
			}
			b.Set(x, y, Cell{Char: HalfBlock, FG: top, BG: bottom})
		}
	}
}
