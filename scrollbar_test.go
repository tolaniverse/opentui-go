package opentui

import (
	"bufio"
	"bytes"
	"errors"
	"testing"
	"time"
)

// barBox is an 8x4 ScrollBox over 12 text rows. Its vertical bar is column
// 7, rows 0-3; the thumb is one cell (4*4/12) and starts at row 0.
func barBox(hideBars bool) (*Renderer, *Node) {
	scroll := ScrollBox(ScrollBoxOptions{
		BoxOptions:     BoxOptions{Style: Style{Width: Cells(8), Height: Cells(4)}},
		HideScrollbars: hideBars,
	}, scrollLines(12)...)
	r := testRenderer(NewNode(Style{}, scroll))
	r.drawFrame(NewBuffer(8, 6))
	return r, scroll
}

func mouse(r *Renderer, typ MouseEventType, x, y int) {
	r.dispatchMouse(&MouseEvent{Type: typ, Button: MouseLeft, X: x, Y: y})
}

func TestScrollbarWinsHitTestOverContent(t *testing.T) {
	r, scroll := barBox(false)
	if hit := r.hitTest(7, 0); hit != scroll {
		t.Fatalf("bar cell hit %p, want scroll box %p", hit, scroll)
	}
	if hit := r.hitTest(1, 0); hit == scroll || hit.text == nil {
		t.Fatal("content cell should hit the text row")
	}
	_, hidden := barBox(true)
	if hidden.scroll.barAt(7, 0) != barNone {
		t.Fatal("hidden scrollbar still hit-tests")
	}
}

func TestScrollbarThumbDrag(t *testing.T) {
	r, scroll := barBox(false)
	mouse(r, MouseDown, 7, 0) // on the thumb
	if r.Selection() != nil {
		t.Fatal("pressing the scrollbar started a text selection")
	}
	mouse(r, MouseDrag, 7, 3)
	if _, y := scroll.ScrollOffset(); y != 8 {
		t.Fatalf("thumb at bottom: y = %d, want 8", y)
	}
	r.drawFrame(NewBuffer(8, 6))
	mouse(r, MouseDrag, 7, 1)
	if _, y := scroll.ScrollOffset(); y != 3 { // round(1*8/3)
		t.Fatalf("thumb at row 1: y = %d, want 3", y)
	}
	mouse(r, MouseUp, 7, 1)
	if scroll.scroll.dragAxis != barNone {
		t.Fatal("drag did not end on mouse up")
	}
	// A later drag that did not start on the bar must not move it.
	scroll.handleScrollMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: 7, Y: 3})
	if _, y := scroll.ScrollOffset(); y != 3 {
		t.Fatalf("drag after release moved scroll to %d", y)
	}
}

func TestScrollbarTrackClickPages(t *testing.T) {
	r, scroll := barBox(false)
	mouse(r, MouseDown, 7, 3) // below the thumb
	if _, y := scroll.ScrollOffset(); y != 3 {
		t.Fatalf("page down: y = %d, want 3 (viewport-1)", y)
	}
	mouse(r, MouseUp, 7, 3)
	r.drawFrame(NewBuffer(8, 6))
	mouse(r, MouseDown, 7, 0) // above the thumb, which is now at row 1
	if _, y := scroll.ScrollOffset(); y != 0 {
		t.Fatalf("page up: y = %d, want 0", y)
	}
}

func TestSelectionClampsToViewportAndAutoScrolls(t *testing.T) {
	r, scroll := barBox(true)
	b := NewBuffer(8, 6)
	mouse(r, MouseDown, 0, 1) // "row1"
	mouse(r, MouseDrag, 3, 5) // two rows below the viewport
	if got := r.SelectedText(); got != "row1\nrow2\nrow3" {
		t.Fatalf("clamped selection = %q", got)
	}
	if box, _, dy := r.autoScrollDelta(); box != scroll || dy != 2 {
		t.Fatalf("auto-scroll delta = %d (box %v)", dy, box == scroll)
	}
	if !r.autoScrollTick() {
		t.Fatal("tick did not scroll")
	}
	r.drawFrame(b) // selection is refreshed against the scrolled layout
	if got := r.SelectedText(); got != "row1\nrow2\nrow3\nrow4\nrow5" {
		t.Fatalf("selection after auto-scroll = %q", got)
	}
	mouse(r, MouseUp, 3, 5)
	if box, _, _ := r.autoScrollDelta(); box != nil {
		t.Fatal("auto-scroll still active after release")
	}
	mouse(r, MouseDown, 0, 1)
	mouse(r, MouseDrag, 3, 2) // inside the viewport: no auto-scroll
	if _, dx, dy := r.autoScrollDelta(); dx != 0 || dy != 0 {
		t.Fatalf("pointer inside viewport gave delta %d,%d", dx, dy)
	}
}

func TestScheduleAutoScrollQueuesOntoRunGoroutine(t *testing.T) {
	r, scroll := barBox(true)
	r.updates, r.done = make(chan func(), 4), make(chan struct{})
	mouse(r, MouseDown, 0, 1)
	mouse(r, MouseDrag, 0, 4) // dispatch schedules the timer
	select {
	case fn := <-r.updates:
		fn()
	case <-time.After(time.Second):
		t.Fatal("auto-scroll timer never queued an update")
	}
	if _, y := scroll.ScrollOffset(); y != 1 {
		t.Fatalf("y = %d after one tick, want 1", y)
	}
	select { // still outside, so the tick schedules the next one
	case fn := <-r.updates:
		fn()
	case <-time.After(time.Second):
		t.Fatal("auto-scroll did not continue")
	}
	if _, y := scroll.ScrollOffset(); y != 2 {
		t.Fatalf("y = %d after two ticks, want 2", y)
	}
	close(r.done)
}

func TestCopyToClipboardOSC52(t *testing.T) {
	var out bytes.Buffer
	r := &Renderer{w: bufio.NewWriter(&out)}
	if err := r.CopyToClipboard("hello", ClipboardSystem); err != nil {
		t.Fatal(err)
	}
	if err := r.CopyToClipboard("hi", ClipboardPrimary); err != nil {
		t.Fatal(err)
	}
	if want := "\x1b]52;c;aGVsbG8=\x1b\\\x1b]52;p;aGk=\x1b\\"; out.String() != want {
		t.Fatalf("got %q want %q", out.String(), want)
	}
	for _, bad := range []string{"", "a\x00b"} {
		if err := r.CopyToClipboard(bad, ClipboardSystem); !errors.Is(err, ErrClipboardText) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}
