package opentui

import (
	"testing"
	"time"
)

func selectableText(content string) *Node {
	return Text(content, TextOptions{})
}

func TestSelectionDragAndClear(t *testing.T) {
	text := selectableText("abcdef")
	root := NewNode(Style{}, text)
	Draw(root, NewBuffer(6, 1))
	r := testRenderer(root)
	finished := 0
	r.SetSelectionHandler(func(*Selection) { finished++ })

	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 1, Y: 0})
	r.dispatchMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: 3, Y: 0})
	r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: 3, Y: 0})
	if got := r.SelectedText(); got != "bcd" {
		t.Fatalf("selected %q", got)
	}
	if finished != 1 || r.Selection().Dragging {
		t.Fatal("selection did not finish")
	}

	b := draw(root, 6, 1)
	cell, _ := b.Get(1, 0)
	if cell.BG != RGB(137, 180, 250) || cell.Char != 'b' {
		t.Fatalf("highlight cell: %+v", cell)
	}

	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 5, Y: 0})
	if r.SelectedText() != "" {
		t.Fatal("new click did not clear previous selection")
	}
}

func TestSelectionAcrossLinesAndNodes(t *testing.T) {
	first := selectableText("one\ntwo")
	second := selectableText("three")
	root := NewNode(Style{}, first, second)
	Draw(root, NewBuffer(6, 3))
	r := testRenderer(root)
	r.startSelection(first, 1, 0)
	r.updateSelection(3, 2)
	if got := r.SelectedText(); got != "ne\ntwo\nthre" {
		t.Fatalf("selected %q", got)
	}
}

func TestSelectionWordAndLineClicks(t *testing.T) {
	text := selectableText("one two")
	root := NewNode(Style{}, text)
	Draw(root, NewBuffer(7, 1))
	r := testRenderer(root)
	click := func(x int) {
		r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: x, Y: 0})
		r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: x, Y: 0})
	}
	click(5)
	click(5)
	if got := r.SelectedText(); got != "two" {
		t.Fatalf("word selection = %q", got)
	}
	click(5)
	if got := r.SelectedText(); got != "one two" {
		t.Fatalf("line selection = %q", got)
	}

	r.lastClickTime = time.Now().Add(-time.Second)
	click(0)
	if got := r.SelectedText(); got != "" {
		t.Fatalf("expired repeat selected %q", got)
	}
}

func TestSelectionFromDecodedDragSequence(t *testing.T) {
	text := selectableText("abcdef")
	root := NewNode(Style{}, text)
	Draw(root, NewBuffer(6, 1))
	r := testRenderer(root)
	d := keyDecoder{}
	for _, event := range d.Feed([]byte("\x1b[<0;2;1M\x1b[<32;4;1M\x1b[<0;4;1m\x1b[ M")) {
		if mouse, ok := event.(mouseInput); ok {
			r.dispatchMouse(mouse.event)
		}
	}
	if got := r.SelectedText(); got != "bcd" {
		t.Fatalf("decoded selection = %q", got)
	}
}

func TestSelectionIgnoresNonSelectableText(t *testing.T) {
	text := Text("secret", TextOptions{NoSelect: true})
	root := NewNode(Style{}, text)
	Draw(root, NewBuffer(6, 1))
	r := testRenderer(root)
	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 0, Y: 0})
	r.dispatchMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: 5, Y: 0})
	if r.SelectedText() != "" {
		t.Fatal("non-selectable text produced a selection")
	}
}
