package opentui

// Rect is an area of the screen in cells. X, Y is the top-left corner.
type Rect struct {
	X, Y, W, H int
}

// Empty reports whether the rect covers no cells.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Contains reports whether cell (x, y) is inside r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// Intersect returns the overlap of r and o (possibly empty).
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := max(r.X, o.X), max(r.Y, o.Y)
	x1, y1 := min(r.X+r.W, o.X+o.W), min(r.Y+r.H, o.Y+o.H)
	if x1 <= x0 || y1 <= y0 {
		return Rect{X: x0, Y: y0}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Inset shrinks r by e on each side.
func (r Rect) Inset(e Edges) Rect {
	return Rect{X: r.X + e.Left, Y: r.Y + e.Top, W: max(r.W-e.Left-e.Right, 0), H: max(r.H-e.Top-e.Bottom, 0)}
}
