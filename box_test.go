package opentui

import (
	"strings"
	"testing"
)

// screen renders the characters of a buffer as text, one line per row,
// for readable snapshot tests.
func screen(b *Buffer) string {
	var sb strings.Builder
	for y := range b.Height() {
		for x := range b.Width() {
			c, _ := b.Get(x, y)
			sb.WriteRune(c.Char)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func draw(root *Node, w, h int) *Buffer {
	b := NewBuffer(w, h)
	Draw(root, b)
	return b
}

func expectScreen(t *testing.T, b *Buffer, want string) {
	t.Helper()
	want = strings.TrimPrefix(want, "\n")
	if got := screen(b); got != want {
		t.Errorf("screen mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestBoxStyles(t *testing.T) {
	root := NewNode(Style{Direction: Row},
		Box(BoxOptions{Border: BorderAll, Style: Style{Width: Cells(4)}}),
		Box(BoxOptions{Border: BorderAll, BorderStyle: BorderDouble, Style: Style{Width: Cells(4)}}),
		Box(BoxOptions{Border: BorderAll, BorderStyle: BorderRounded, Style: Style{Width: Cells(4)}}),
		Box(BoxOptions{Border: BorderAll, BorderStyle: BorderHeavy, Style: Style{Width: Cells(4)}}),
	)
	expectScreen(t, draw(root, 16, 3), `
┌──┐╔══╗╭──╮┏━━┓
│  │║  ║│  │┃  ┃
└──┘╚══╝╰──╯┗━━┛
`)
}

func TestBoxTitles(t *testing.T) {
	root := NewNode(Style{},
		Box(BoxOptions{Border: BorderAll, Title: "Hi", Style: Style{Height: Cells(2)}}),
		Box(BoxOptions{Border: BorderAll, Title: "Hi", TitleAlign: TitleCenter, Style: Style{Height: Cells(2)}}),
		Box(BoxOptions{Border: BorderAll, BottomTitle: "Hi", BottomTitleAlign: TitleRight, Style: Style{Height: Cells(2)}}),
		Box(BoxOptions{Border: BorderAll, Title: "Too long", Style: Style{Height: Cells(2), Width: Cells(10)}}),
	)
	expectScreen(t, draw(root, 10, 8), `
┌─Hi─────┐
└────────┘
┌───Hi───┐
└────────┘
┌────────┐
└─────Hi─┘
┌────────┐
└────────┘
`)
}

func TestPartialBorders(t *testing.T) {
	root := NewNode(Style{Direction: Row, Gap: 1},
		Box(BoxOptions{Border: SideLeft, Style: Style{Width: Cells(2)}}),
		Box(BoxOptions{Border: SideTop | SideLeft | SideRight, Style: Style{Width: Cells(4)}}),
		Box(BoxOptions{Border: SideBottom | SideLeft, Style: Style{Width: Cells(4)}}),
	)
	// Left-only: one full line. Top+sides: verticals run to the bottom.
	// Bottom+left: the vertical runs to the top.
	expectScreen(t, draw(root, 12, 3), `
│  ┌──┐ │   
│  │  │ │   
│  │  │ └───
`)
}

func TestBoxFillAndChildrenInside(t *testing.T) {
	bg := RGB(10, 20, 30)
	txt := Text("ok", TextOptions{})
	box := Box(BoxOptions{Border: BorderAll, Background: bg, Style: Style{Padding: XY(1, 0)}}, txt)
	b := draw(NewNode(Style{}, box), 8, 3)
	expectScreen(t, b, `
┌──────┐
│ ok   │
└──────┘
`)
	if c, _ := b.Get(5, 1); c.BG != bg {
		t.Errorf("inside should be filled, bg=%v", c.BG)
	}
	if c, _ := b.Get(2, 1); c.BG != bg || c.Char != 'o' {
		t.Errorf("text over a filled box keeps the box bg, got %+v", c)
	}
}

func TestOverflowHiddenClips(t *testing.T) {
	long := Text("abcdefghij", TextOptions{Style: Style{Width: Cells(10)}})
	clip := Box(BoxOptions{Border: BorderAll, Style: Style{Width: Cells(6), Height: Cells(3), Overflow: Hidden, AlignItems: AlignStart}}, long)
	free := Box(BoxOptions{Border: BorderAll, Style: Style{Width: Cells(6), Height: Cells(3), AlignItems: AlignStart}},
		Text("abcdefghij", TextOptions{Style: Style{Width: Cells(10)}}))
	root := NewNode(Style{AlignItems: AlignStart}, clip, free)
	expectScreen(t, draw(root, 12, 6), `
┌────┐      
│abcd│      
└────┘      
┌────┐      
│abcdefghij 
└────┘      
`)
}

func TestZIndexOrder(t *testing.T) {
	under := Box(BoxOptions{Background: RGB(1, 1, 1), ZIndex: 0, Style: Style{Position: Absolute, Left: Cells(0), Top: Cells(0), Width: Cells(3), Height: Cells(1)}},
		Text("AAA", TextOptions{}))
	over := Box(BoxOptions{Background: RGB(2, 2, 2), ZIndex: 5, Style: Style{Position: Absolute, Left: Cells(1), Top: Cells(0), Width: Cells(3), Height: Cells(1)}},
		Text("BBB", TextOptions{}))
	// over is added first but has the higher ZIndex, so it paints last.
	root := NewNode(Style{}, over, under)
	expectScreen(t, draw(root, 5, 1), "\nABBB \n")
}

func TestClipStackNests(t *testing.T) {
	b := NewBuffer(10, 1)
	b.PushClip(Rect{X: 2, W: 6, H: 1})
	b.PushClip(Rect{X: 0, W: 4, H: 1}) // intersected → x 2..3
	b.DrawText("xxxxxxxxxx", 0, 0, White, Transparent, AttrNone)
	b.PopClip()
	b.PopClip()
	if got := screen(b); got != "  xx      \n" {
		t.Errorf("got %q", got)
	}
}
