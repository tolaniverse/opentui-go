package opentui

import "testing"

func TestFlexWrapMovesChildrenAcrossRowsAndResize(t *testing.T) {
	children := []*TextNode{
		Text("one", TextOptions{Style: Style{Width: Cells(4)}}),
		Text("two", TextOptions{Style: Style{Width: Cells(4)}}),
		Text("red", TextOptions{Style: Style{Width: Cells(4)}}),
	}
	root := NewNode(Style{Direction: Row, FlexWrap: FlexWrapLines, Gap: 1}, NodeHandles(children)...)
	b := draw(root, 9, 4)
	if got := screen(b); got != "one  two \n         \nred      \n         \n" {
		t.Fatalf("wrapped paint:\n%s", got)
	}
	if children[2].Rect().Y != 2 {
		t.Fatalf("third child at %+v", children[2].Rect())
	}
	draw(root, 14, 4)
	if children[2].Rect().Y != 0 {
		t.Fatalf("resize did not reflow: %+v", children[2].Rect())
	}
}
