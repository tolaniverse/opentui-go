package opentui

// Attr is a set of text style flags, stored as bits.
//
// Port of `TextAttributes` in upstream ansi.zig. Upstream also packs a
// hyperlink ID into the upper 24 bits; links arrive in a later release.
type Attr uint8

const (
	AttrBold Attr = 1 << iota
	AttrDim
	AttrItalic
	AttrUnderline
	AttrBlink
	AttrInverse
	AttrHidden
	AttrStrikethrough

	AttrNone Attr = 0
)

// Cell is one character position on screen.
//
// Port of `Cell` in upstream buffer.zig. v0.1.0 stores one rune per cell and
// treats every rune as one column wide; wide characters and grapheme
// clusters come in a later release.
type Cell struct {
	Char rune
	FG   RGBA
	BG   RGBA
	Attr Attr
}

// clearChar marks cells that have never been painted. Nothing draws it, so
// the first frame after a clear differs from every real cell and repaints
// all of them. Upstream renderer.zig uses the same trick (CLEAR_CHAR).
const clearChar rune = '਀'

// White is the default foreground of a cleared cell, as upstream.
var White = RGB(255, 255, 255)

// Buffer is a width×height grid of cells, stored row by row in one slice.
//
// Port of the core of `OptimizedBuffer` in upstream buffer.zig. Upstream
// uses a "struct of arrays" (one array per field) for SIMD row comparisons;
// we start with a plain slice of structs for clarity.
type Buffer struct {
	width, height int
	cells         []Cell
	// backdrop is blended under transparent cells, like upstream
	// blendBackdropColor (the renderer sets it to its background color).
	backdrop *RGBA
	// clips is a stack of clip rectangles ("scissor stack" upstream).
	// Writes outside the top rect are dropped. Each pushed rect is
	// intersected with the one below, so clips only ever get smaller.
	clips []Rect
}

// NewBuffer allocates a buffer filled with spaces on a transparent background.
func NewBuffer(width, height int) *Buffer {
	b := &Buffer{}
	b.Resize(width, height)
	return b
}

func (b *Buffer) Width() int  { return b.width }
func (b *Buffer) Height() int { return b.height }

// Resize changes the size and clears the contents.
func (b *Buffer) Resize(width, height int) {
	width, height = max(width, 0), max(height, 0)
	b.width, b.height = width, height
	b.clips = b.clips[:0]
	if n := width * height; cap(b.cells) >= n {
		b.cells = b.cells[:n]
	} else {
		b.cells = make([]Cell, n)
	}
	b.Clear(Transparent)
}

// SetBackdrop sets the color blended under fully transparent cells.
func (b *Buffer) SetBackdrop(c RGBA) { b.backdrop = &c }

// Clear fills every cell with a space, white foreground and bg.
func (b *Buffer) Clear(bg RGBA) { b.fill(' ', bg) }

func (b *Buffer) fill(ch rune, bg RGBA) {
	c := Cell{Char: ch, FG: White, BG: bg}
	for i := range b.cells {
		b.cells[i] = c
	}
}

func (b *Buffer) inBounds(x, y int) bool {
	if n := len(b.clips); n > 0 {
		return b.clips[n-1].Contains(x, y)
	}
	return x >= 0 && y >= 0 && x < b.width && y < b.height
}

// PushClip restricts drawing to r (intersected with the current clip).
// Every PushClip needs a matching PopClip.
// Port of pushScissorRect in upstream buffer.zig.
func (b *Buffer) PushClip(r Rect) {
	b.clips = append(b.clips, r.Intersect(b.Clip()))
}

// PopClip undoes the last PushClip.
func (b *Buffer) PopClip() {
	if len(b.clips) > 0 {
		b.clips = b.clips[:len(b.clips)-1]
	}
}

// Clip returns the area drawing is currently allowed in.
func (b *Buffer) Clip() Rect {
	if n := len(b.clips); n > 0 {
		return b.clips[n-1]
	}
	return Rect{W: b.width, H: b.height}
}

// Get returns the cell at (x, y). ok is false when out of bounds.
func (b *Buffer) Get(x, y int) (c Cell, ok bool) {
	if x < 0 || y < 0 || x >= b.width || y >= b.height {
		return Cell{}, false
	}
	return b.cells[y*b.width+x], true
}

// Set overwrites a cell without blending. Out-of-bounds writes are ignored.
func (b *Buffer) Set(x, y int, c Cell) {
	if b.inBounds(x, y) {
		b.cells[y*b.width+x] = c
	}
}

// SetBlended draws c over the existing cell, honoring alpha in c.FG / c.BG.
// Port of `blendCells` in upstream buffer.zig.
func (b *Buffer) SetBlended(x, y int, c Cell) {
	if !b.inBounds(x, y) {
		return
	}
	i := y*b.width + x
	b.cells[i] = b.blendCells(c, b.cells[i])
}

func (b *Buffer) blendCells(over, dest Cell) Cell {
	fgAlpha, bgAlpha := over.FG.A < 255, over.BG.A < 255
	if !fgAlpha && !bgAlpha {
		return over // opaque: plain overwrite
	}

	bg := over.BG
	if bgAlpha {
		bg = blend(over.BG, dest.BG, b.backdrop)
	}

	// A translucent space over existing text keeps that text visible and
	// tints it, instead of erasing it. This is how overlays "dim" content.
	keep := over.Char == ' ' && dest.Char != 0 && dest.Char != ' ' && dest.Char != clearChar
	if keep {
		return Cell{Char: dest.Char, FG: blend(over.BG, dest.FG, b.backdrop), BG: bg, Attr: dest.Attr}
	}

	fg := over.FG
	if fgAlpha {
		fg = blend(over.FG, bg, b.backdrop)
	}
	return Cell{Char: over.Char, FG: fg, BG: bg, Attr: over.Attr}
}

// DrawText writes text starting at (x, y), one rune per cell, clipped to the
// buffer. It does not wrap. Translucent colors blend with what is underneath.
func (b *Buffer) DrawText(text string, x, y int, fg, bg RGBA, attr Attr) {
	clip := b.Clip()
	if y < clip.Y || y >= clip.Y+clip.H {
		return
	}
	for _, r := range text {
		if x >= clip.X+clip.W {
			return
		}
		if r < 0x20 || r == 0x7f { // control characters would corrupt the screen
			r = ' '
		}
		b.SetBlended(x, y, Cell{Char: r, FG: fg, BG: bg, Attr: attr})
		x++
	}
}

// FillRect paints the background of a rectangle, clipped to the buffer.
// An opaque bg erases what was there; a translucent bg tints it.
func (b *Buffer) FillRect(x, y, w, h int, bg RGBA) {
	area := Rect{X: x, Y: y, W: w, H: h}.Intersect(b.Clip())
	x0, y0, x1, y1 := area.X, area.Y, area.X+area.W, area.Y+area.H
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			b.SetBlended(xx, yy, Cell{Char: ' ', FG: White, BG: bg})
		}
	}
}
