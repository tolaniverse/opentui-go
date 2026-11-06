package opentui

import (
	"fmt"
	"testing"
)

func scrollLines(count int) []*Node {
	lines := make([]*Node, count)
	for i := range lines {
		lines[i] = Text(fmt.Sprintf("row%d", i), TextOptions{})
	}
	return lines
}

func TestScrollBoxLayoutClampAndPaint(t *testing.T) {
	scroll := ScrollBox(ScrollBoxOptions{
		BoxOptions:     BoxOptions{Style: Style{Width: Cells(8), Height: Cells(3)}},
		HideScrollbars: true,
	}, scrollLines(8)...)
	root := NewNode(Style{}, scroll)
	b := draw(root, 8, 3)
	if width, height := scroll.ScrollSize(); width != 8 || height != 8 {
		t.Fatalf("content size = %dx%d", width, height)
	}
	if got := screen(b); got != "row0    \nrow1    \nrow2    \n" {
		t.Fatalf("initial:\n%s", got)
	}
	if !scroll.ScrollTo(0, 99) {
		t.Fatal("scroll did not move")
	}
	b = draw(root, 8, 3)
	if x, y := scroll.ScrollOffset(); x != 0 || y != 5 {
		t.Fatalf("offset = %d,%d", x, y)
	}
	if got := screen(b); got != "row5    \nrow6    \nrow7    \n" {
		t.Fatalf("scrolled:\n%s", got)
	}
}

func TestScrollBoxWheelAndKeyboard(t *testing.T) {
	scroll := ScrollBox(ScrollBoxOptions{
		BoxOptions:     BoxOptions{Style: Style{Width: Cells(8), Height: Cells(3)}},
		HideScrollbars: true, WheelStep: 2,
	}, scrollLines(8)...)
	root := NewNode(Style{}, scroll)
	Draw(root, NewBuffer(8, 3))
	r := testRenderer(root)
	r.dispatchMouse(&MouseEvent{Type: MouseScroll, X: 1, Y: 1, Scroll: &ScrollInfo{Direction: ScrollDown, Delta: 1}})
	if _, y := scroll.ScrollOffset(); y != 2 {
		t.Fatalf("wheel y = %d", y)
	}
	r.Focus(scroll)
	r.dispatchInput(keyInput{press("pagedown", "\x1b[6~")})
	if _, y := scroll.ScrollOffset(); y != 4 {
		t.Fatalf("page y = %d", y)
	}
	r.dispatchInput(keyInput{press("end", "\x1b[F")})
	if _, y := scroll.ScrollOffset(); y != 5 {
		t.Fatalf("end y = %d", y)
	}
}

func TestScrollBoxStickyEndFollowsGrowth(t *testing.T) {
	scroll := ScrollBox(ScrollBoxOptions{
		BoxOptions: BoxOptions{Style: Style{Width: Cells(8), Height: Cells(2)}},
		StickyEnd:  true, HideScrollbars: true,
	}, scrollLines(3)...)
	root := NewNode(Style{}, scroll)
	Draw(root, NewBuffer(8, 2))
	if _, y := scroll.ScrollOffset(); y != 1 {
		t.Fatalf("initial sticky y = %d", y)
	}
	scroll.Add(Text("row3", TextOptions{}))
	Draw(root, NewBuffer(8, 2))
	if _, y := scroll.ScrollOffset(); y != 2 {
		t.Fatalf("grown sticky y = %d", y)
	}
	scroll.ScrollTo(0, 0)
	scroll.Add(Text("row4", TextOptions{}))
	Draw(root, NewBuffer(8, 2))
	if _, y := scroll.ScrollOffset(); y != 0 {
		t.Fatalf("sticky followed while away: %d", y)
	}
}

func TestScrollBoxOnScrollAndChildrenFacade(t *testing.T) {
	children := scrollLines(4)
	var positions [][2]int
	scroll := ScrollBox(ScrollBoxOptions{
		BoxOptions:     BoxOptions{Style: Style{Width: Cells(5), Height: Cells(2)}},
		HideScrollbars: true,
		OnScroll:       func(x, y int) { positions = append(positions, [2]int{x, y}) },
	}, children...)
	root := NewNode(Style{}, scroll)
	Draw(root, NewBuffer(5, 2))
	if got := scroll.Children(); len(got) != 4 || got[0] != children[0] {
		t.Fatalf("children: %v", got)
	}
	scroll.ScrollBy(0, 1)
	if len(positions) != 1 || positions[0] != [2]int{0, 1} {
		t.Fatalf("positions: %v", positions)
	}
	if !scroll.Remove(children[0]) || len(scroll.Children()) != 3 {
		t.Fatal("remove did not delegate")
	}
}

func TestScrollBoxHitTestingClipsOffscreenChildren(t *testing.T) {
	lines := scrollLines(5)
	scroll := ScrollBox(ScrollBoxOptions{
		BoxOptions:     BoxOptions{Style: Style{Width: Cells(8), Height: Cells(2)}},
		HideScrollbars: true,
	}, lines...)
	root := NewNode(Style{}, scroll)
	Draw(root, NewBuffer(8, 2))
	scroll.ScrollTo(0, 3)
	Draw(root, NewBuffer(8, 2))
	r := testRenderer(root)
	if hit := r.hitTest(1, 0); hit != lines[3] {
		t.Fatalf("hit = %p, want row3 %p", hit, lines[3])
	}
}

func TestScrollBoxPaintsScrollbar(t *testing.T) {
	scroll := ScrollBox(ScrollBoxOptions{
		BoxOptions: BoxOptions{Style: Style{Width: Cells(6), Height: Cells(3)}},
	}, scrollLines(6)...)
	b := draw(NewNode(Style{}, scroll), 6, 3)
	cell, ok := b.Get(5, 0)
	if !ok || cell.BG != RGB(137, 180, 250) {
		t.Fatalf("thumb cell: %+v", cell)
	}
	cell, _ = b.Get(5, 2)
	if cell.BG != RGB(69, 71, 90) {
		t.Fatalf("track cell: %+v", cell)
	}
}
