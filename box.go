package opentui

// BorderStyle picks the line characters of a border.
type BorderStyle uint8

const (
	BorderSingle  BorderStyle = iota // ┌─┐ (upstream default)
	BorderDouble                     // ╔═╗
	BorderRounded                    // ╭─╮
	BorderHeavy                      // ┏━┓
)

// BorderChars are the runes used to draw a border.
// Port of BorderCharacters in upstream lib/border.ts (corners and lines;
// the T-junctions and cross arrive with tables).
type BorderChars struct {
	TopLeft, TopRight, BottomLeft, BottomRight rune
	Horizontal, Vertical                       rune
}

var borderChars = [...]BorderChars{
	BorderSingle:  {'┌', '┐', '└', '┘', '─', '│'},
	BorderDouble:  {'╔', '╗', '╚', '╝', '═', '║'},
	BorderRounded: {'╭', '╮', '╰', '╯', '─', '│'},
	BorderHeavy:   {'┏', '┓', '┗', '┛', '━', '┃'},
}

// Sides is a set of box sides.
type Sides uint8

const (
	SideTop Sides = 1 << iota
	SideRight
	SideBottom
	SideLeft

	BorderNone Sides = 0
	BorderAll        = SideTop | SideRight | SideBottom | SideLeft
)

func (s Sides) has(side Sides) bool { return s&side != 0 }

// TitleAlign positions a title inside the top or bottom border.
type TitleAlign uint8

const (
	TitleLeft TitleAlign = iota
	TitleCenter
	TitleRight
)

// BoxOptions configures Box. Port of upstream BoxOptions (renderables/Box.ts).
type BoxOptions struct {
	ID     string
	Style  Style
	ZIndex int

	Background RGBA // Transparent (zero) = no fill
	NoFill     bool // upstream shouldFill:false

	Border       Sides
	BorderStyle  BorderStyle
	BorderColor  RGBA         // zero value = white, the upstream default
	CustomBorder *BorderChars // overrides BorderStyle

	Title            string
	TitleAlign       TitleAlign
	TitleColor       RGBA // zero value = BorderColor, as upstream
	BottomTitle      string
	BottomTitleAlign TitleAlign
}

// Box returns a node that paints a background, a border and titles, and
// lays out its children inside. Each border side takes one cell of layout
// space, as upstream sets a Yoga border of 1 per drawn side.
func Box(o BoxOptions, children ...*Node) *Node {
	n := NewNode(o.Style, children...)
	n.ID, n.ZIndex = o.ID, o.ZIndex
	n.border = Edges{
		Top:    b2i(o.Border.has(SideTop)),
		Right:  b2i(o.Border.has(SideRight)),
		Bottom: b2i(o.Border.has(SideBottom)),
		Left:   b2i(o.Border.has(SideLeft)),
	}
	if o.BorderColor == (RGBA{}) {
		o.BorderColor = White
	}
	if o.TitleColor == (RGBA{}) {
		o.TitleColor = o.BorderColor
	}
	n.paint = func(b *Buffer, n *Node) { drawBox(b, n.rect, &o) }
	return n
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

// drawBox paints a box into r. Port of drawVisibleBox in upstream
// buffer.zig. Clipping is left to the buffer's clip stack.
func drawBox(b *Buffer, r Rect, o *BoxOptions) {
	if r.Empty() {
		return
	}
	sides := o.Border
	fill := !o.NoFill && o.Background.A > 0
	if sides == BorderNone && !fill {
		return // a layout-only box draws nothing
	}

	if fill {
		inner := r.Inset(Edges{
			Top: b2i(sides.has(SideTop)), Right: b2i(sides.has(SideRight)),
			Bottom: b2i(sides.has(SideBottom)), Left: b2i(sides.has(SideLeft)),
		})
		b.FillRect(inner.X, inner.Y, inner.W, inner.H, o.Background)
	}
	if sides == BorderNone {
		return
	}

	ch := borderChars[o.BorderStyle]
	if o.CustomBorder != nil {
		ch = *o.CustomBorder
	}
	top, right, bottom, left := sides.has(SideTop), sides.has(SideRight), sides.has(SideBottom), sides.has(SideLeft)
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W-1, r.Y+r.H-1

	set := func(x, y int, c rune) {
		b.SetBlended(x, y, Cell{Char: c, FG: o.BorderColor, BG: o.Background})
	}

	topTitle := titleSpan(o.Title, top, r, o.TitleAlign)
	bottomTitle := titleSpan(o.BottomTitle, bottom, r, o.BottomTitleAlign)

	horizontal := func(y int, span [2]int, cornerL, cornerR rune) {
		for x := x0; x <= x1; x++ {
			if span[1] > span[0] && x >= span[0] && x < span[1] {
				continue // the title goes here
			}
			c := ch.Horizontal
			if x == x0 && left {
				c = cornerL
			} else if x == x1 && right {
				c = cornerR
			}
			set(x, y, c)
		}
	}
	if top {
		horizontal(y0, topTitle, ch.TopLeft, ch.TopRight)
	}
	if bottom {
		horizontal(y1, bottomTitle, ch.BottomLeft, ch.BottomRight)
	}

	// A vertical line runs into a missing horizontal side instead of
	// stopping short, so e.g. a left-only border is one full line.
	leftOnly := left && !top && !bottom
	rightOnly := right && !top && !bottom
	extendTop := leftOnly || rightOnly || (bottom && !top && (left || right))
	extendBottom := leftOnly || rightOnly || (top && !bottom && (left || right))
	vy0, vy1 := y0, y1
	if top && !extendTop {
		vy0++
	}
	if bottom && !extendBottom {
		vy1--
	}
	for y := vy0; y <= vy1; y++ {
		if left {
			set(x0, y, ch.Vertical)
		}
		if right {
			set(x1, y, ch.Vertical)
		}
	}

	if topTitle[1] > topTitle[0] {
		b.DrawText(o.Title, topTitle[0], y0, o.TitleColor, o.Background, AttrNone)
	}
	if bottomTitle[1] > bottomTitle[0] {
		b.DrawText(o.BottomTitle, bottomTitle[0], y1, o.TitleColor, o.Background, AttrNone)
	}
}

// titleSpan returns the [start, end) columns of a title, or an empty span
// when there is no title, no border on that side, or not enough room
// (a title needs 4 spare cells). Port of computeBoxTitleLayout.
func titleSpan(title string, side bool, r Rect, align TitleAlign) [2]int {
	n := len([]rune(title)) // v0.2: one cell per rune
	if n == 0 || !side || r.W < n+4 {
		return [2]int{}
	}
	const pad = 2
	x := r.X + pad
	switch align {
	case TitleCenter:
		x = r.X + (r.W-n)/2
	case TitleRight:
		x = r.X + r.W - pad - n
	}
	return [2]int{x, x + n}
}
