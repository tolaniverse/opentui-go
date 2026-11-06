package opentui

import (
	"fmt"
	"testing"
)

func sgrMouse(code, x, y int, press bool) []byte {
	final := 'm'
	if press {
		final = 'M'
	}
	return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", code, x+1, y+1, final))
}

func mouseEvents(events []terminalEvent) []*MouseEvent {
	var out []*MouseEvent
	for _, event := range events {
		if mouse, ok := event.(mouseInput); ok {
			out = append(out, mouse.event)
		}
	}
	return out
}

func TestMouseDecoderSGRButtonsScrollAndModifiers(t *testing.T) {
	d := keyDecoder{}
	tests := []struct {
		code      int
		press     bool
		typ       MouseEventType
		button    int
		direction ScrollDirection
	}{
		{0, true, MouseDown, MouseLeft, ""},
		{0, false, MouseUp, MouseLeft, ""},
		{1, true, MouseDown, MouseMiddle, ""},
		{2, true, MouseDown, MouseRight, ""},
		{64, true, MouseScroll, 0, ScrollUp},
		{65, true, MouseScroll, 0, ScrollDown},
		{66, true, MouseScroll, 0, ScrollLeft},
		{67, true, MouseScroll, 0, ScrollRight},
	}
	for _, tc := range tests {
		got := mouseEvents(d.Feed(sgrMouse(tc.code, 10, 5, tc.press)))
		if len(got) != 1 || got[0].Type != tc.typ || got[0].Button != tc.button || got[0].X != 10 || got[0].Y != 5 {
			t.Fatalf("code %d: %+v", tc.code, got)
		}
		if tc.direction != "" && (got[0].Scroll == nil || got[0].Scroll.Direction != tc.direction) {
			t.Fatalf("code %d scroll: %+v", tc.code, got[0].Scroll)
		}
	}
	got := mouseEvents(d.Feed(sgrMouse(28, 1, 2, true)))
	if len(got) != 1 || !got[0].Modifiers.Shift || !got[0].Modifiers.Alt || !got[0].Modifiers.Ctrl {
		t.Fatalf("modifiers: %+v", got)
	}
}

func TestMouseDecoderSGRPressMotionRelease(t *testing.T) {
	d := keyDecoder{}
	got := mouseEvents(d.Feed([]byte("\x1b[<0;5;7M\x1b[<32;10;7M\x1b[<0;10;7m")))
	if len(got) != 3 || got[0].Type != MouseDown || got[1].Type != MouseDrag || got[2].Type != MouseUp {
		t.Fatalf("sequence: %+v", got)
	}
}

func TestMouseDecoderDragAndSplitReads(t *testing.T) {
	d := keyDecoder{}
	d.Feed(sgrMouse(0, 1, 1, true))
	drag := mouseEvents(d.Feed(sgrMouse(32, 3, 2, true)))
	if len(drag) != 1 || drag[0].Type != MouseDrag {
		t.Fatalf("drag: %+v", drag)
	}
	d.Feed(sgrMouse(0, 3, 2, false))
	move := mouseEvents(d.Feed(sgrMouse(32, 4, 2, true)))
	if len(move) != 1 || move[0].Type != MouseMove {
		t.Fatalf("move: %+v", move)
	}

	d = keyDecoder{}
	sequence := sgrMouse(0, 20, 10, true)
	if got := d.Feed(sequence[:5]); len(got) != 0 {
		t.Fatalf("partial emitted: %#v", got)
	}
	got := mouseEvents(d.Feed(sequence[5:]))
	if len(got) != 1 || got[0].X != 20 || got[0].Y != 10 {
		t.Fatalf("split: %+v", got)
	}
}

func TestMouseDecoderX10PreservesRawCoordinates(t *testing.T) {
	d := keyDecoder{}
	input := []byte{0x1b, '[', 'M', 32, 128, 129}
	got := mouseEvents(d.Feed(input))
	if len(got) != 1 || got[0].Type != MouseDown || got[0].X != 95 || got[0].Y != 96 {
		t.Fatalf("X10: %+v", got)
	}
}

func TestMouseRoutingBubblesAndClickFocuses(t *testing.T) {
	input := Input(InputOptions{Style: Style{Width: Cells(5), Height: Cells(1)}})
	parent := NewNode(Style{Width: Cells(10), Height: Cells(3)}, input)
	root := NewNode(Style{}, parent)
	b := NewBuffer(10, 3)
	Draw(root, b)
	r := testRenderer(root)
	var order []string
	input.SetMouseEventHandler(MouseDown, func(event *MouseEvent) {
		order = append(order, "input")
		if event.LocalX != 2 || event.LocalY != 0 {
			t.Errorf("local input = %d,%d", event.LocalX, event.LocalY)
		}
	})
	parent.SetMouseHandler(func(event *MouseEvent) { order = append(order, "parent") })
	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 2, Y: 0})
	if len(order) != 2 || order[0] != "input" || order[1] != "parent" {
		t.Fatalf("order: %v", order)
	}
	if r.Focused() != input {
		t.Fatal("left click did not focus input")
	}
}

func TestMousePreventDefaultStopsAutoFocus(t *testing.T) {
	input := Input(InputOptions{Style: Style{Width: Cells(5), Height: Cells(1)}})
	root := NewNode(Style{}, input)
	Draw(root, NewBuffer(5, 1))
	r := testRenderer(root)
	input.SetMouseEventHandler(MouseDown, func(event *MouseEvent) { event.PreventDefault() })
	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 0, Y: 0})
	if r.Focused() != nil {
		t.Fatal("prevented click focused input")
	}
}

func TestMouseDragCaptureAndDrop(t *testing.T) {
	source := NewNode(Style{Width: Cells(5), Height: Cells(1)})
	target := NewNode(Style{Width: Cells(5), Height: Cells(1)})
	root := NewNode(Style{Direction: Row}, source, target)
	Draw(root, NewBuffer(10, 1))
	r := testRenderer(root)
	var sourceTypes, targetTypes []MouseEventType
	source.SetMouseHandler(func(event *MouseEvent) { sourceTypes = append(sourceTypes, event.Type) })
	target.SetMouseEventHandler(MouseDrop, func(event *MouseEvent) {
		targetTypes = append(targetTypes, event.Type)
		if event.Source != source {
			t.Error("drop source mismatch")
		}
	})
	r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: 1, Y: 0})
	r.dispatchMouse(&MouseEvent{Type: MouseDrag, Button: MouseLeft, X: 6, Y: 0})
	r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: 6, Y: 0})
	if len(sourceTypes) < 4 || sourceTypes[len(sourceTypes)-2] != MouseDragEnd || sourceTypes[len(sourceTypes)-1] != MouseUp {
		t.Fatalf("source events: %v", sourceTypes)
	}
	if len(targetTypes) != 1 || targetTypes[0] != MouseDrop {
		t.Fatalf("target events: %v", targetTypes)
	}
}
