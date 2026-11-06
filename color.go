package opentui

import (
	"fmt"
	"strconv"
	"strings"
)

// ColorIntent records how a color should be emitted to the terminal.
//
// Port of `ColorIntent` in upstream ansi.zig.
type ColorIntent uint8

const (
	// IntentRGB emits a literal 24-bit color (38;2;r;g;b).
	IntentRGB ColorIntent = iota
	// IntentIndexed emits a palette slot (38;5;n).
	IntentIndexed
	// IntentDefault emits the terminal's own default color (39 / 49).
	IntentDefault
)

// RGBA is a color plus the intent used to emit it.
//
// Upstream packs this into [4]u16 so two colors compare with one integer
// check. In Go a struct of plain fields is already comparable with ==, so we
// keep readable fields instead.
//
// R, G, B, A are always a usable RGB "snapshot", even for indexed and default
// colors, so alpha blending works on every color.
type RGBA struct {
	R, G, B, A uint8
	Intent     ColorIntent
	Slot       uint8 // palette index; meaningful only when Intent == IntentIndexed
}

// Transparent is fully see-through. As a background it emits the terminal default.
var Transparent = RGBA{}

// RGB returns an opaque literal color.
func RGB(r, g, b uint8) RGBA { return RGBA{R: r, G: g, B: b, A: 255} }

// RGBAOf returns a literal color with alpha.
func RGBAOf(r, g, b, a uint8) RGBA { return RGBA{R: r, G: g, B: b, A: a} }

// Indexed returns a 256-palette color. Its RGB snapshot uses the standard
// xterm palette so blending still has numbers to work with.
func Indexed(index uint8) RGBA {
	c := Ansi256(index)
	c.Intent = IntentIndexed
	c.Slot = index
	return c
}

// Default returns the terminal's default color. snapshot is the RGB value used
// for blending (the terminal never tells us its real default).
func Default(snapshot RGBA) RGBA {
	snapshot.Intent = IntentDefault
	snapshot.Slot = 0
	return snapshot
}

// ansi16 is the standard 16-color palette (upstream ANSI16_RGB).
var ansi16 = [16][3]uint8{
	{0x00, 0x00, 0x00}, {0x80, 0x00, 0x00}, {0x00, 0x80, 0x00}, {0x80, 0x80, 0x00},
	{0x00, 0x00, 0x80}, {0x80, 0x00, 0x80}, {0x00, 0x80, 0x80}, {0xc0, 0xc0, 0xc0},
	{0x80, 0x80, 0x80}, {0xff, 0x00, 0x00}, {0x00, 0xff, 0x00}, {0xff, 0xff, 0x00},
	{0x00, 0x00, 0xff}, {0xff, 0x00, 0xff}, {0x00, 0xff, 0xff}, {0xff, 0xff, 0xff},
}

var cubeLevels = [6]uint8{0, 95, 135, 175, 215, 255}

// Ansi256 returns the RGB value of xterm palette entry index as a literal color.
// Port of `fallbackAnsi256Color`.
func Ansi256(index uint8) RGBA {
	i := int(index)
	switch {
	case i < 16:
		return RGB(ansi16[i][0], ansi16[i][1], ansi16[i][2])
	case i < 232:
		c := i - 16
		return RGB(cubeLevels[(c/36)%6], cubeLevels[(c/6)%6], cubeLevels[c%6])
	default:
		g := uint8(8 + (i-232)*10)
		return RGB(g, g, g)
	}
}

// Hex parses "#rgb", "#rgba", "#rrggbb" or "#rrggbbaa" (the "#" is optional).
//
// Upstream hexToRgb warns and falls back to magenta on bad input; in Go we
// return an error instead and let the caller decide.
func Hex(s string) (RGBA, error) {
	h := strings.TrimPrefix(s, "#")
	if len(h) == 3 || len(h) == 4 {
		var b strings.Builder
		for i := 0; i < len(h); i++ {
			b.WriteByte(h[i])
			b.WriteByte(h[i])
		}
		h = b.String()
	}
	if len(h) != 6 && len(h) != 8 {
		return RGBA{}, fmt.Errorf("opentui: invalid hex color %q", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return RGBA{}, fmt.Errorf("opentui: invalid hex color %q", s)
	}
	if len(h) == 6 {
		return RGB(uint8(v>>16), uint8(v>>8), uint8(v)), nil
	}
	return RGBAOf(uint8(v>>24), uint8(v>>16), uint8(v>>8), uint8(v)), nil
}

// MustHex is Hex that panics on error. Use it for constants.
func MustHex(s string) RGBA {
	c, err := Hex(s)
	if err != nil {
		panic(err)
	}
	return c
}

// String formats the color like upstream rgbToHex ("#rrggbb" or "#rrggbbaa").
func (c RGBA) String() string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func mulDiv255(a, b uint32) uint32 { return (a*b + 127) / 255 }

func roundDiv(n, d uint32) uint8 { return uint8((n + d/2) / d) }

// blend composites src over dst ("source-over"), using integer math.
// If dst is fully transparent and a backdrop is given, blend onto the backdrop.
//
// Port of `blendColors` in upstream buffer.zig. The result is always a
// literal RGB color: once mixed, a palette slot no longer describes it.
func blend(src, dst RGBA, backdrop *RGBA) RGBA {
	sa := uint32(src.A)
	if sa == 0 {
		return dst
	}
	if dst.A == 0 && backdrop != nil {
		dst = *backdrop
	}
	if sa == 255 {
		return RGB(src.R, src.G, src.B)
	}
	da := uint32(dst.A)
	inv := 255 - sa
	outA := sa + mulDiv255(da, inv)
	if outA == 0 {
		return RGBAOf(0, 0, 0, 0)
	}
	if da == 255 {
		mix := func(s, d uint8) uint8 { return uint8((uint32(s)*sa + uint32(d)*inv + 127) / 255) }
		return RGB(mix(src.R, dst.R), mix(src.G, dst.G), mix(src.B, dst.B))
	}
	mix := func(s, d uint8) uint8 {
		return roundDiv(uint32(s)*sa+mulDiv255(uint32(d)*da, inv), outA)
	}
	return RGBAOf(mix(src.R, dst.R), mix(src.G, dst.G), mix(src.B, dst.B), uint8(outA))
}
