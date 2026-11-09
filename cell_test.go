package opentui

import (
	"bytes"
	"strings"
	"testing"
)

func TestCellGraphemeAndLinkAPI(t *testing.T) {
	plain := Cell{Char: 'a'}
	if plain.Grapheme() != "a" || plain.Link() != "" {
		t.Fatalf("plain cell: %q %q", plain.Grapheme(), plain.Link())
	}
	if (Cell{}).Grapheme() != "" || (Cell{Char: 'x', Continuation: true}).Grapheme() != "" {
		t.Fatal("empty and continuation cells must have no grapheme")
	}
	wide := Cell{}.WithGrapheme("界")
	if wide.Char != '界' || wide.Grapheme() != "界" || wide.width() != 2 {
		t.Fatalf("single-rune wide cell: %+v width %d", wide, wide.width())
	}
	if wide.ext != (Cell{}).ext {
		t.Fatal("a single-rune glyph must not need an interned handle")
	}
	emoji := Cell{}.WithGrapheme("👩🏽‍💻").WithLink("https://a")
	if emoji.Grapheme() != "👩🏽‍💻" || emoji.Link() != "https://a" || emoji.width() != 2 {
		t.Fatalf("cluster+link cell: %q %q %d", emoji.Grapheme(), emoji.Link(), emoji.width())
	}
	if emoji.WithLink("").Link() != "" || emoji.WithLink("").Grapheme() != "👩🏽‍💻" {
		t.Fatal("removing the link must keep the grapheme")
	}
	if got := (Cell{Char: 'x'}).WithLink("https://x\x1b]0;y").Link(); got != "https://x]0;y" {
		t.Fatalf("WithLink must sanitize: %q", got)
	}
}

// Diff rendering compares cells with ==, so cells with equal content must
// be equal even when their strings were built separately.
func TestCellsWithEqualContentCompareEqual(t *testing.T) {
	a := Cell{Char: 'e', FG: White}.WithGrapheme("é").WithLink("https://example.com/" + "a")
	b := Cell{Char: 'e', FG: White}.WithGrapheme(strings.Join([]string{"e", "́"}, "")).WithLink("https://example.com/a")
	if a != b {
		t.Fatal("equal cells built from different strings compare unequal")
	}
	if a == b.WithLink("https://example.com/b") {
		t.Fatal("different links compare equal")
	}
}

// Frame encoding must not allocate: it runs for every frame.
func TestEncodeFrameDoesNotAllocate(t *testing.T) {
	current, next := NewBuffer(80, 24), NewBuffer(80, 24)
	for y := 0; y < 24; y++ {
		next.DrawText("╭─ box │ text 界 👩🏽‍💻 é", 0, y, RGB(200, 100, 50), RGB(10, 20, 30), AttrBold)
	}
	var out bytes.Buffer
	out.Grow(1 << 20)
	allocs := testing.AllocsPerRun(20, func() {
		out.Reset()
		current.fill(clearChar, Transparent)
		encodeFrame(&out, current, next, FrameOptions{colors: newColorEncoder(Color256, Transparent)})
	})
	if allocs != 0 {
		t.Fatalf("encodeFrame allocated %.0f times per frame", allocs)
	}
}
