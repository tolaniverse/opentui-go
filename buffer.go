package opentui

import (
	"unicode/utf8"
	"unique"

	"github.com/rivo/uniseg"
)

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

// Cell is one terminal position. A glyph wider than one cell occupies a
// leading cell and continuation cells. Port of `Cell` in upstream
// buffer.zig.
//
// Char is the glyph's first rune. A multi-rune grapheme cluster (an
// accented letter, an emoji sequence) and an OSC 8 link are stored in one
// interned handle (Go's unique package), so comparing cells, which diff
// rendering does for every cell of every frame, costs one pointer compare
// for them. Read them with Grapheme and Link; set them with WithGrapheme
// and WithLink.
type Cell struct {
	Char         rune
	FG           RGBA
	BG           RGBA
	Attr         Attr
	Continuation bool
	ext          unique.Handle[cellExt] // zero: single-rune glyph, no link
}

// cellExt is the rarely used part of a cell, interned per distinct value.
type cellExt struct {
	cluster string // the full grapheme cluster when it has several runes
	link    string
}

func makeExt(cluster, link string) unique.Handle[cellExt] {
	if cluster == "" && link == "" {
		return unique.Handle[cellExt]{}
	}
	return unique.Make(cellExt{cluster, link})
}

func (c Cell) extValue() cellExt {
	if c.ext == (unique.Handle[cellExt]{}) {
		return cellExt{}
	}
	return c.ext.Value()
}

// Grapheme returns the text the cell shows: its grapheme cluster, or ""
// for a continuation or unpainted cell.
func (c Cell) Grapheme() string {
	if c.Continuation || c.Char == 0 {
		return ""
	}
	if g := c.extValue().cluster; g != "" {
		return g
	}
	return string(c.Char)
}

// Link returns the URL of the OSC 8 hyperlink over the cell, or "".
func (c Cell) Link() string { return c.extValue().link }

// WithGrapheme returns c showing the grapheme cluster g (Char becomes its
// first rune). Draw text with Buffer.DrawText to have it segmented for you.
func (c Cell) WithGrapheme(g string) Cell {
	r, size := utf8.DecodeRuneInString(g)
	c.Char = r
	cluster := ""
	if size < len(g) {
		cluster = g
	}
	c.ext = makeExt(cluster, c.Link())
	return c
}

// WithLink returns c with an OSC 8 hyperlink to url ("" removes it).
func (c Cell) WithLink(url string) Cell {
	c.ext = makeExt(c.extValue().cluster, sanitizeLink(url))
	return c
}

// cluster is the stored multi-rune cluster, or "" when Char alone is the
// glyph.
func (c Cell) cluster() string { return c.extValue().cluster }

// width is how many cells the glyph occupies. The fast path is small
// enough to inline: it runs for every cell write, often several times.
func (c Cell) width() int {
	if c.ext == (unique.Handle[cellExt]{}) && c.Char < 0x1100 {
		return 1
	}
	return c.slowWidth()
}

func (c Cell) slowWidth() int {
	if g := c.cluster(); g != "" {
		return max(uniseg.StringWidth(g), 1)
	}
	return runeWidth(c.Char)
}

// runeWidth is the cell width of a single-rune glyph. Nothing below U+1100
// is wide, which keeps Latin and most other text off the uniseg path.
func runeWidth(r rune) int {
	if r < 0x1100 {
		return 1
	}
	var buf [utf8.UTFMax]byte
	return max(uniseg.StringWidth(string(utf8.AppendRune(buf[:0], r))), 1)
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
		width := c.width()
		if !b.inBounds(x+width-1, y) {
			return // never store half a wide glyph
		}
		b.eraseWideAt(x, y)
		for j := 1; j < width; j++ {
			b.eraseWideAt(x+j, y)
			b.cells[y*b.width+x+j] = Cell{Continuation: true, FG: c.FG, BG: c.BG, Attr: c.Attr}
		}
		b.cells[y*b.width+x] = c
	}
}

// eraseWideAt clears all cells of an old multi-cell glyph when any is replaced.
func (b *Buffer) eraseWideAt(x, y int) {
	if c := b.cells[y*b.width+x]; !c.Continuation && c.width() < 2 {
		return // the common case: a narrow glyph, nothing to erase
	}
	start := x
	for start > 0 && b.cells[y*b.width+start].Continuation {
		start--
	}
	lead := b.cells[y*b.width+start]
	width := lead.width()
	if width < 2 {
		return
	}
	for col := start; col < min(start+width, b.width); col++ {
		old := b.cells[y*b.width+col]
		b.cells[y*b.width+col] = Cell{Char: ' ', FG: White, BG: old.BG}
	}
}

// SetBlended draws c over the existing cell, honoring alpha in c.FG / c.BG.
// Port of `blendCells` in upstream buffer.zig.
func (b *Buffer) SetBlended(x, y int, c Cell) {
	if !b.inBounds(x, y) {
		return
	}
	i := y*b.width + x
	width := c.width()
	if !b.inBounds(x+width-1, y) {
		return
	}
	// A translucent space tints text rather than erasing it. Keep both
	// halves of an existing wide glyph intact when a translucent fill paints
	// each cell independently.
	if c.Char == ' ' && c.BG.A < 255 {
		old := b.cells[i]
		if old.Continuation {
			old.BG = blend(c.BG, old.BG, b.backdrop)
			b.cells[i] = old
			return
		}
		if old.width() > 1 {
			b.cells[i] = b.blendCells(c, old)
			return
		}
	}
	b.eraseWideAt(x, y)
	if width > 1 {
		for j := 1; j < width; j++ {
			b.eraseWideAt(x+j, y)
			b.cells[i+j] = Cell{Continuation: true, FG: c.FG, BG: c.BG, Attr: c.Attr}
		}
	}
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
		return Cell{Char: dest.Char, FG: blend(over.BG, dest.FG, b.backdrop), BG: bg, Attr: dest.Attr, ext: dest.ext}
	}

	fg := over.FG
	if fgAlpha {
		fg = blend(over.FG, bg, b.backdrop)
	}
	return Cell{Char: over.Char, FG: fg, BG: bg, Attr: over.Attr, ext: over.ext}
}

// DrawText writes grapheme clusters starting at (x, y), clipped to the
// buffer. It does not wrap. Translucent colors blend with what is underneath.
func (b *Buffer) DrawText(text string, x, y int, fg, bg RGBA, attr Attr) {
	b.DrawTextLink(text, "", x, y, fg, bg, attr)
}

// DrawTextLink is DrawText with an OSC 8 hyperlink to url over the text
// ("" for none). Control bytes in url are dropped so it cannot end the
// escape sequence early.
func (b *Buffer) DrawTextLink(text, url string, x, y int, fg, bg RGBA, attr Attr) {
	url = sanitizeLink(url)
	link := makeExt("", url) // one handle for the whole run of text
	clip := b.Clip()
	if y < clip.Y || y >= clip.Y+clip.H {
		return
	}
	if asciiOnly(text) {
		for i := 0; i < len(text) && x < clip.X+clip.W; i++ {
			if x >= clip.X {
				r := rune(text[i])
				if r < 0x20 || r == 0x7f {
					r = ' '
				}
				b.SetBlended(x, y, Cell{Char: r, FG: fg, BG: bg, Attr: attr, ext: link})
			}
			x++
		}
		return
	}
	graphemes := uniseg.NewGraphemes(text)
	for graphemes.Next() {
		glyph := graphemes.Str()
		width := graphemes.Width()
		r, _ := utf8.DecodeRuneInString(glyph)
		if r < 0x20 || r == 0x7f { // control characters would corrupt the screen
			r, glyph, width = ' ', " ", 1
		}
		if width <= 0 {
			continue
		}
		if x >= clip.X+clip.W {
			return
		}
		if x < clip.X || x+width > clip.X+clip.W {
			x += width // never paint a partial grapheme
			continue
		}
		cell := Cell{Char: r, FG: fg, BG: bg, Attr: attr, ext: link}
		if len(glyph) != utf8.RuneLen(r) {
			// Only multi-rune clusters need a handle; a single rune, even a
			// wide one, is fully described by Char.
			cell.ext = makeExt(glyph, url)
		}
		b.SetBlended(x, y, cell)
		x += width
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

// drawCursor shows a text cursor at x, y by inverting the cell already
// painted there, so it is visible over any glyph, a blank, or a cell after
// the end of the text. On a wide glyph it inverts every cell of the glyph.
// Redrawing the glyph instead fails for zero-width clusters and for a
// translucent background, where a space only tints what is under it.
func (b *Buffer) drawCursor(x, y int) {
	if !b.inBounds(x, y) {
		return
	}
	row := y * b.width
	start := x
	for start > 0 && b.cells[row+start].Continuation {
		start--
	}
	for col := start; col < b.width && (col == start || b.cells[row+col].Continuation); col++ {
		b.cells[row+col].Attr |= AttrInverse
	}
}

// recolor sets the colors of cells [x0, x1) on row y, keeping their
// content, within the current clip. It highlights a painted range, such
// as an editor selection.
func (b *Buffer) recolor(x0, x1, y int, fg, bg RGBA) {
	for x := x0; x < x1; x++ {
		if b.inBounds(x, y) {
			c := &b.cells[y*b.width+x]
			c.FG, c.BG = fg, bg
		}
	}
}

// sanitizeLink drops C0/C1 control bytes and DEL from a hyperlink URL.
func sanitizeLink(url string) string {
	if url == "" {
		return ""
	}
	clean := true
	for _, r := range url {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			clean = false
			break
		}
	}
	if clean {
		return url
	}
	out := make([]rune, 0, len(url))
	for _, r := range url {
		if !(r < 0x20 || (r >= 0x7f && r <= 0x9f)) {
			out = append(out, r)
		}
	}
	return string(out)
}
