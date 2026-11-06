package opentui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// These benchmarks measure the Go implementation, not upstream Zig. Keep the
// screen size and scene stable so results can be compared between revisions.
func BenchmarkEncodeFrame(b *testing.B) {
	const width, height = 80, 24
	fg, bg := RGB(205, 214, 244), RGB(30, 30, 46)
	paint := func(buf *Buffer) {
		buf.Clear(bg)
		for y := 0; y < height; y++ {
			buf.DrawText("The quick brown fox jumps over the lazy dog", 2, y, fg, bg, AttrNone)
		}
	}
	for _, mode := range []string{"unchanged", "one-cell", "full"} {
		b.Run(mode, func(b *testing.B) {
			current, next := NewBuffer(width, height), NewBuffer(width, height)
			paint(next)
			if mode != "full" {
				paint(current)
			}
			var out bytes.Buffer
			out.Grow(width * height * 24)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				out.Reset()
				switch mode {
				case "one-cell":
					// Toggle the same cell after the previous encode updated current.
					cell := next.cells[width+2]
					if cell.Char == 'X' {
						cell.Char = 'T'
					} else {
						cell.Char = 'X'
					}
					next.cells[width+2] = cell
				case "full":
					current.fill(clearChar, bg)
				}
				encodeFrame(&out, current, next, FrameOptions{})
			}
		})
	}
}

func BenchmarkInputPaintUnicode(b *testing.B) {
	n := Input(InputOptions{Value: strings.Repeat("界e\u0301", 500), Style: Style{Width: Cells(40)}})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	r := testRenderer(root)
	r.Focus(n)
	buf := NewBuffer(40, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Draw(root, buf)
	}
}

func BenchmarkInputPasteUnicode(b *testing.B) {
	text := strings.Repeat("界e\u0301", 500)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		n := Input(InputOptions{MaxLength: 1000})
		n.InsertText(text)
	}
}

func BenchmarkDrawDocument(b *testing.B) {
	rows := make([]*Node, 200)
	for i := range rows {
		rows[i] = Text(fmt.Sprintf("Line %03d: selectable content for a scrolling terminal document", i), TextOptions{})
	}
	scroll := ScrollBox(ScrollBoxOptions{BoxOptions: BoxOptions{Style: Style{FlexGrow: 1}}}, rows...)
	root := NewNode(Style{}, scroll)
	buf := NewBuffer(80, 24)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Clear(Transparent)
		scroll.ScrollTo(0, i%177)
		Draw(root, buf)
	}
}
