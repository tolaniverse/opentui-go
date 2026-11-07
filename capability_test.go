package opentui

import (
	"bytes"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestDetectColorMode(t *testing.T) {
	for _, tc := range []struct {
		vars map[string]string
		want ColorMode
	}{
		{map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor"}, ColorTrueColor},
		{map[string]string{"TERM": "xterm-256color", "COLORTERM": "24bit"}, ColorTrueColor},
		{map[string]string{"TERM": "xterm-256color"}, Color256},
		{map[string]string{"TERM": "tmux-256COLOR"}, Color256},
		{map[string]string{"TERM": "xterm", "WT_SESSION": "abc"}, ColorTrueColor},
		{map[string]string{"TERM": "linux"}, Color16},
		{map[string]string{"TERM": "vt100"}, Color16},
		{map[string]string{"TERM": "xterm"}, ColorTrueColor}, // unknown: RGB, as upstream
		{map[string]string{}, ColorTrueColor},
		{map[string]string{"TERM": "xterm-256color", "COLORTERM": "truecolor", "NO_COLOR": "1"}, ColorNone},
		{map[string]string{"TERM": "linux", "NO_COLOR": ""}, Color16}, // empty NO_COLOR is ignored
	} {
		if got := DetectColorMode(env(tc.vars)); got != tc.want {
			t.Errorf("%v: got %v, want %v", tc.vars, got, tc.want)
		}
	}
}

func TestNearestPaletteIndex(t *testing.T) {
	for _, tc := range []struct {
		c    RGBA
		size int
		want uint8
	}{
		{RGB(95, 135, 175), 256, 67}, // exact cube entry
		{RGB(128, 128, 128), 256, 8}, // exact match in both the base 16 and the ramp: lower index wins
		{RGB(255, 0, 0), 256, 9},
		{RGB(240, 240, 240), 256, 255}, // gray ramp 238
		{RGB(250, 10, 10), 16, 9},
		{RGB(0, 0, 100), 16, 4},
		{RGB(137, 180, 250), 16, 7}, // a light blue has no close basic color: light gray
	} {
		if got := nearestIndex(tc.c, tc.size); got != tc.want {
			t.Errorf("nearest(%v, %d) = %d, want %d", tc.c, tc.size, got, tc.want)
		}
	}
}

func encodeIn(mode ColorMode, background RGBA, cells ...Cell) string {
	current, next := newPair(len(cells), 1)
	for x, c := range cells {
		next.Set(x, 0, c)
	}
	var out bytes.Buffer
	encodeFrame(&out, current, next, FrameOptions{colors: newColorEncoder(mode, background)})
	return out.String()
}

func TestColorModesEncodeColors(t *testing.T) {
	red := Cell{Char: 'r', FG: RGB(250, 10, 10), BG: RGB(0, 0, 100)}
	for _, tc := range []struct {
		mode      ColorMode
		has, lack []string
	}{
		{ColorTrueColor, []string{"\x1b[38;2;250;10;10m", "\x1b[48;2;0;0;100m"}, nil},
		{Color256, []string{"\x1b[38;5;9m", "\x1b[48;5;17m"}, []string{"38;2;", "48;2;"}},
		{Color16, []string{"\x1b[91m", "\x1b[44m"}, []string{"38;", "48;"}},
		{ColorNone, []string{"\x1b[39m", "\x1b[49m"}, []string{"38;", "48;", "\x1b[91m", "\x1b[44m"}},
	} {
		got := encodeIn(tc.mode, Transparent, red)
		for _, want := range tc.has {
			if !strings.Contains(got, want) {
				t.Errorf("%v: %q missing %q", tc.mode, got, want)
			}
		}
		for _, bad := range tc.lack {
			if strings.Contains(got, bad) {
				t.Errorf("%v: %q contains %q", tc.mode, got, bad)
			}
		}
	}
}

func TestColor16MapsPaletteSlots(t *testing.T) {
	got := encodeIn(Color16, Transparent,
		Cell{Char: 'a', FG: Indexed(3), BG: Transparent},
		Cell{Char: 'b', FG: Indexed(196), BG: Transparent}) // 196 = (255,0,0) -> bright red
	if !strings.Contains(got, "\x1b[33m") || !strings.Contains(got, "\x1b[91m") || strings.Contains(got, "38;5;") {
		t.Fatalf("palette slots in 16-color mode: %q", got)
	}
	if got := encodeIn(Color256, Transparent, Cell{Char: 'a', FG: Indexed(196), BG: Transparent}); !strings.Contains(got, "\x1b[38;5;196m") {
		t.Fatalf("palette slot in 256-color mode: %q", got)
	}
}

func TestColorNoneInvertsHighlightedCells(t *testing.T) {
	bg := RGB(30, 30, 46)
	plain := Cell{Char: 'p', FG: White, BG: bg}              // the renderer background
	selected := Cell{Char: 's', FG: White, BG: RGB(1, 2, 3)} // a highlight
	cursor := Cell{Char: 'c', FG: White, BG: bg, Attr: AttrInverse}
	got := encodeIn(ColorNone, bg, plain, selected, cursor)
	// Expected runs: "p" plain; "s" inverse; "c" inverse (its own attribute).
	if strings.Count(got, "\x1b[7m") != 2 {
		t.Fatalf("want 2 inverse runs, got %q", got)
	}
	if i := strings.Index(got, "p"); strings.Contains(got[:i], "\x1b[7m") {
		t.Fatalf("plain cell inverted: %q", got)
	}
}

func TestPaletteIndexIsCached(t *testing.T) {
	e := newColorEncoder(Color256, Transparent)
	var out bytes.Buffer
	for i := 0; i < 3; i++ {
		e.emit(&out, RGB(10, 20, 30), false)
		e.emit(&out, RGB(10, 20, 30), true)
	}
	e.emit(&out, RGB(10, 20, 30), false)
	if len(e.nearest) != 1 {
		t.Fatalf("cache has %d entries, want 1", len(e.nearest))
	}
	e16 := newColorEncoder(Color16, Transparent)
	e16.emit(&out, RGB(10, 20, 30), false)
	if len(e16.nearest) != 1 {
		t.Fatalf("16-color cache has %d entries, want 1", len(e16.nearest))
	}
}
