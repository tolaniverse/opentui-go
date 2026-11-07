package opentui

import (
	"strings"

	"github.com/tolaniverse/opentui-go/ansi"
)

// ColorMode is how many colors the renderer may send to the terminal.
type ColorMode uint8

const (
	// ColorAuto (the zero value) detects the mode from the environment
	// when the renderer starts; see DetectColorMode.
	ColorAuto ColorMode = iota
	// ColorTrueColor sends 24-bit colors (SGR 38;2 / 48;2).
	ColorTrueColor
	// Color256 maps each color to the nearest xterm-256 palette entry.
	Color256
	// Color16 maps each color to the nearest of the 16 basic ANSI colors.
	Color16
	// ColorNone sends no colors. Cells whose background differs from the
	// renderer background are drawn inverse, so highlights such as a
	// selection or a focused field stay visible.
	ColorNone
)

func (m ColorMode) String() string {
	switch m {
	case ColorTrueColor:
		return "truecolor"
	case Color256:
		return "256"
	case Color16:
		return "16"
	case ColorNone:
		return "none"
	}
	return "auto"
}

// DetectColorMode picks a color mode from environment variables, read
// with getenv (pass os.Getenv). It follows upstream terminal.zig:
//
//   - COLORTERM=truecolor or 24bit, or WT_SESSION set: ColorTrueColor
//   - TERM containing "256color": Color256
//   - anything else: ColorTrueColor, as upstream sends RGB by default
//
// It adds two rules upstream does not have:
//
//   - NO_COLOR set to anything non-empty: ColorNone (https://no-color.org)
//   - TERM=linux or vt*: Color16, since those consoles cannot show RGB
//
// It does not query the terminal.
func DetectColorMode(getenv func(string) string) ColorMode {
	term := strings.ToLower(getenv("TERM"))
	colorterm := strings.ToLower(getenv("COLORTERM"))
	switch {
	case getenv("NO_COLOR") != "":
		return ColorNone
	case colorterm == "truecolor" || colorterm == "24bit" || getenv("WT_SESSION") != "":
		return ColorTrueColor
	case strings.Contains(term, "256color"):
		return Color256
	case term == "linux" || strings.HasPrefix(term, "vt"):
		return Color16
	}
	return ColorTrueColor
}

// colorEncoder writes colors for one terminal's color mode. It caches
// nearest-palette lookups, as upstream cachedNearestPaletteIndex does.
type colorEncoder struct {
	mode       ColorMode
	background RGBA // the renderer background, for ColorNone highlights
	nearest    map[uint32]uint8
}

func newColorEncoder(mode ColorMode, background RGBA) *colorEncoder {
	if mode == ColorAuto {
		mode = ColorTrueColor
	}
	return &colorEncoder{mode: mode, background: background, nearest: map[uint32]uint8{}}
}

func rgb24(c RGBA) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }

func distanceSquared(a, b RGBA) int {
	dr, dg, db := int(a.R)-int(b.R), int(a.G)-int(b.G), int(a.B)-int(b.B)
	return dr*dr + dg*dg + db*db
}

// nearestIndex returns the palette index in [0, size) closest to c in RGB
// distance; ties go to the lower index, as upstream.
func nearestIndex(c RGBA, size int) uint8 {
	best, bestDistance := 0, int(^uint(0)>>1)
	for i := 0; i < size; i++ {
		if d := distanceSquared(c, Ansi256(uint8(i))); d < bestDistance {
			best, bestDistance = i, d
		}
	}
	return uint8(best)
}

// paletteIndex is the nearest xterm-256 entry for c, cached; size 16
// restricts the search to the basic colors.
func (e *colorEncoder) paletteIndex(c RGBA, size int) uint8 {
	key := rgb24(c) | uint32(size&0x100)<<16 // separate 16- and 256-color entries
	if i, ok := e.nearest[key]; ok {
		return i
	}
	i := nearestIndex(c, size)
	e.nearest[key] = i
	return i
}

// emit writes the SGR code for one color in the encoder's mode. A nil
// encoder means truecolor.
func (e *colorEncoder) emit(w ansi.Writer, c RGBA, background bool) {
	defaultColor := func() {
		if background {
			w.WriteString(ansi.BgDefault)
		} else {
			w.WriteString(ansi.FgDefault)
		}
	}
	mode := ColorTrueColor
	if e != nil {
		mode = e.mode
	}
	switch {
	case c.Intent == IntentDefault || (background && c.A == 0) || mode == ColorNone:
		defaultColor()
	case c.Intent == IntentIndexed && mode != Color16:
		if background {
			ansi.BgIndexed(w, c.Slot)
		} else {
			ansi.FgIndexed(w, c.Slot)
		}
	case mode == ColorTrueColor:
		if background {
			ansi.BgRGB(w, c.R, c.G, c.B)
		} else {
			ansi.FgRGB(w, c.R, c.G, c.B)
		}
	case mode == Color256:
		if background {
			ansi.BgIndexed(w, e.paletteIndex(c, 256))
		} else {
			ansi.FgIndexed(w, e.paletteIndex(c, 256))
		}
	default: // Color16
		i := c.Slot
		if c.Intent != IntentIndexed || i >= 16 {
			rgb := c
			if c.Intent == IntentIndexed {
				rgb = Ansi256(c.Slot)
			}
			i = e.paletteIndex(rgb, 16)
		}
		ansi.Basic(w, i, background)
	}
}

// highlight reports whether ColorNone should invert cell to keep a
// colored background visible.
func (e *colorEncoder) highlight(cell Cell) bool {
	if e == nil || e.mode != ColorNone || cell.BG.Intent == IntentDefault || cell.BG.A == 0 {
		return false
	}
	return rgb24(cell.BG) != rgb24(e.background) || e.background.A == 0
}
