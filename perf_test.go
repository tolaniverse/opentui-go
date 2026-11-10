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
		rows[i] = Text(fmt.Sprintf("Line %03d: selectable content for a scrolling terminal document", i), TextOptions{}).Node
	}
	scroll := ScrollBox(ScrollBoxOptions{BoxOptions: BoxOptions{Style: Style{FlexGrow: 1}}}, NodeHandles(rows)...)
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

// BenchmarkTextareaTypeAndDraw is one keystroke plus a full frame on a
// 2,000-line document, cursor at the end.
func BenchmarkTextareaTypeAndDraw(b *testing.B) {
	for _, kind := range []string{"ascii", "unicode", "ascii-wrap", "unicode-wrap"} {
		b.Run(kind, func(b *testing.B) {
			line := "The quick brown fox jumps over the lazy dog"
			if strings.HasPrefix(kind, "unicode") {
				line = "界é quick brown 👩🏽‍💻 jumps over the lazy dog"
			}
			wrap := WrapNone // explicit: Textarea wraps by default
			if strings.HasSuffix(kind, "-wrap") {
				wrap, line = WrapWord, strings.Repeat(line+" ", 4) // ~180 cells: 3 rows at 80
			}
			n := Textarea(TextareaOptions{
				Wrap:  wrap,
				Value: strings.Repeat(line+"\n", 2000),
				Style: Style{Width: Cells(80), Height: Cells(24)},
			})
			root := NewNode(Style{AlignItems: AlignStart}, n)
			r := testRenderer(root)
			r.Focus(n)
			buf := NewBuffer(80, 24)
			keys := []*KeyEvent{press("x", "x"), press("backspace", "\x7f")}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				k := *keys[i%2]
				n.keyHandler(&k)
				Draw(root, buf)
			}
		})
	}
}

// BenchmarkDrawWrappedDocument lays out and paints 200 word-wrapped
// paragraphs in a scrolling box, the wrapped counterpart of
// BenchmarkDrawDocument.
func BenchmarkDrawWrappedDocument(b *testing.B) {
	para := strings.Repeat("Selectable content for a scrolling terminal document, wrapped by word. ", 3)
	rows := make([]*Node, 200)
	for i := range rows {
		rows[i] = Text(fmt.Sprintf("%03d %s", i, para), TextOptions{Wrap: WrapWord}).Node
	}
	scroll := ScrollBox(ScrollBoxOptions{BoxOptions: BoxOptions{Style: Style{FlexGrow: 1}}}, NodeHandles(rows)...)
	root := NewNode(Style{}, scroll)
	buf := NewBuffer(80, 24)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Clear(Transparent)
		scroll.ScrollTo(0, i%500)
		Draw(root, buf)
	}
}

// BenchmarkCtrlWLongLine deletes words from the end of a 2,500-cluster
// non-ASCII line; stepping by inputPrev made this quadratic in v0.9.
func BenchmarkCtrlWLongLine(b *testing.B) {
	line := strings.Repeat("界界界界 ", 500)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		n := Textarea(TextareaOptions{Value: line})
		n.keyHandler(key("w", "\x17", "ctrl"))
	}
}

// BenchmarkUnicodeScreen paints and diffs a full 80x24 screen of CJK text
// with emoji ZWJ sequences, the case where cells carry grapheme data.
func BenchmarkUnicodeScreen(b *testing.B) {
	line := strings.Repeat("日本語のテキスト👩🏽‍💻", 4)
	rows := make([]*Node, 24)
	for i := range rows {
		rows[i] = Text(line, TextOptions{}).Node
	}
	root := NewNode(Style{}, NodeHandles(rows)...)
	current, next := NewBuffer(80, 24), NewBuffer(80, 24)
	var out bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next.Clear(Transparent)
		Draw(root, next)
		out.Reset()
		if i%2 == 0 {
			current.fill(clearChar, Transparent) // force a full repaint every other frame
		}
		encodeFrame(&out, current, next, FrameOptions{})
	}
}
