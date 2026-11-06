package opentui

import (
	"strconv"
	"strings"
)

// MouseEventType identifies a decoded pointer action.
type MouseEventType string

const (
	MouseDown    MouseEventType = "down"
	MouseUp      MouseEventType = "up"
	MouseMove    MouseEventType = "move"
	MouseDrag    MouseEventType = "drag"
	MouseDragEnd MouseEventType = "drag-end"
	MouseDrop    MouseEventType = "drop"
	MouseOver    MouseEventType = "over"
	MouseOut     MouseEventType = "out"
	MouseScroll  MouseEventType = "scroll"
)

// Mouse button numbers follow terminal protocols and upstream OpenTUI.
const (
	MouseLeft = iota
	MouseMiddle
	MouseRight
)

// ScrollDirection is a terminal wheel direction.
type ScrollDirection string

const (
	ScrollUp    ScrollDirection = "up"
	ScrollDown  ScrollDirection = "down"
	ScrollLeft  ScrollDirection = "left"
	ScrollRight ScrollDirection = "right"
)

// MouseModifiers are modifier keys reported with a mouse event.
type MouseModifiers struct{ Shift, Alt, Ctrl bool }

// ScrollInfo accompanies MouseScroll events.
type ScrollInfo struct {
	Direction ScrollDirection
	Delta     int
}

// MouseEvent is one decoded and routed terminal pointer event. X and Y are
// zero-based screen cells; LocalX and LocalY are relative to CurrentTarget.
type MouseEvent struct {
	Type               MouseEventType
	Button             int
	X, Y               int
	LocalX, LocalY     int
	Modifiers          MouseModifiers
	Scroll             *ScrollInfo
	Target             *Node
	CurrentTarget      *Node
	Source             *Node
	IsDragging         bool
	defaultPrevented   bool
	propagationStopped bool
}

func (e *MouseEvent) PreventDefault()          { e.defaultPrevented = true }
func (e *MouseEvent) DefaultPrevented() bool   { return e.defaultPrevented }
func (e *MouseEvent) StopPropagation()         { e.propagationStopped = true }
func (e *MouseEvent) PropagationStopped() bool { return e.propagationStopped }

type mouseDecoder struct{ pressed map[int]bool }

func (d *mouseDecoder) parse(raw []byte) *MouseEvent {
	if len(raw) < 3 || raw[0] != 0x1b || raw[1] != '[' {
		return nil
	}
	if raw[2] == 'M' {
		if len(raw) != 6 {
			return nil
		}
		return d.decode(int(raw[3])-32, int(raw[4])-33, int(raw[5])-33, true, false)
	}
	if raw[2] != '<' || len(raw) < 7 {
		return nil
	}
	final := raw[len(raw)-1]
	if final != 'M' && final != 'm' {
		return nil
	}
	parts := strings.Split(string(raw[3:len(raw)-1]), ";")
	if len(parts) != 3 {
		return nil
	}
	values := [3]int{}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		values[i] = value
	}
	return d.decode(values[0], values[1]-1, values[2]-1, final == 'M', true)
}

func (d *mouseDecoder) decode(code, x, y int, press bool, sgr bool) *MouseEvent {
	button := code & 3
	motion := code&32 != 0
	scroll := code&64 != 0
	e := &MouseEvent{
		Button: button, X: x, Y: y,
		Modifiers: MouseModifiers{Shift: code&4 != 0, Alt: code&8 != 0, Ctrl: code&16 != 0},
	}
	if motion {
		e.Type = MouseMove
		if button != 3 && len(d.pressed) > 0 {
			e.Type = MouseDrag
		}
		if button == 3 {
			e.Button = -1
		}
		return e
	}
	if scroll && press {
		directions := [...]ScrollDirection{ScrollUp, ScrollDown, ScrollLeft, ScrollRight}
		e.Type, e.Button = MouseScroll, 0
		e.Scroll = &ScrollInfo{Direction: directions[button], Delta: 1}
		return e
	}
	if sgr {
		if press {
			e.Type = MouseDown
			if button != 3 {
				if d.pressed == nil {
					d.pressed = make(map[int]bool)
				}
				d.pressed[button] = true
			}
		} else {
			e.Type = MouseUp
			delete(d.pressed, button)
		}
		if button == 3 {
			e.Button = 0
		}
		return e
	}
	if button == 3 {
		e.Type, e.Button = MouseUp, 0
	} else {
		e.Type = MouseDown
	}
	return e
}

func (r *Renderer) dispatchMouse(raw *MouseEvent) {
	if r.opts.NoMouse || raw == nil {
		return
	}
	if r.mouseHandler != nil {
		r.mouseHandler(raw)
	}
	target := r.hitTest(raw.X, raw.Y)
	if raw.Type == MouseScroll && target == nil {
		target = r.focused
	}
	left := raw.Button == MouseLeft
	if left && raw.Type == MouseDrag && r.selection != nil && r.selection.Dragging {
		r.updateSelection(raw.X, raw.Y)
		r.scheduleAutoScroll()
		if target != nil {
			raw.IsDragging = true
			r.sendMouse(target, raw)
		}
		return
	}
	if left && raw.Type == MouseUp && r.selection != nil && r.selection.Dragging {
		r.updateSelection(raw.X, raw.Y)
		r.finishSelection()
		if target != nil {
			raw.IsDragging = true
			r.sendMouse(target, raw)
		}
		r.pressedMouse = nil
		return
	}

	if (raw.Type == MouseMove || raw.Type == MouseDrag) && target != r.hoveredMouse {
		if r.hoveredMouse != nil {
			r.sendMouse(r.hoveredMouse, cloneMouse(raw, MouseOut))
		}
		oldCapture := r.capturedMouse
		r.hoveredMouse = target
		if target != nil {
			over := cloneMouse(raw, MouseOver)
			over.Source = oldCapture
			r.sendMouse(target, over)
		}
	}

	if left && raw.Type == MouseDown {
		r.pressedMouse = target
	}
	if left && raw.Type == MouseDrag && r.pressedMouse != nil {
		r.capturedMouse = r.pressedMouse
		target = r.capturedMouse
	}
	if left && raw.Type == MouseUp && r.capturedMouse != nil {
		source := r.capturedMouse
		end := cloneMouse(raw, MouseDragEnd)
		end.IsDragging = true
		r.sendMouse(source, end)
		up := cloneMouse(raw, MouseUp)
		up.IsDragging = true
		r.sendMouse(source, up)
		r.capturedMouse = nil
		r.pressedMouse = nil
		if target != nil && target != source && !isDescendant(target, source) {
			drop := cloneMouse(raw, MouseDrop)
			drop.Source = source
			r.sendMouse(target, drop)
		}
		return
	}

	if target != nil {
		raw.Target = target
		r.sendMouse(target, raw)
		if left && raw.Type == MouseDown && target.Selectable && target.text != nil && !raw.Modifiers.Ctrl && !raw.DefaultPrevented() {
			r.startSelection(target, raw.X, raw.Y)
		}
		if raw.Type == MouseDown && left && !raw.DefaultPrevented() && !r.opts.NoAutoFocus {
			for n := target; n != nil; n = n.parent {
				if n.Focusable {
					r.Focus(n)
					break
				}
			}
		}
	}
	if left && raw.Type == MouseDown && (target == nil || !target.Selectable || target.text == nil) && !raw.DefaultPrevented() {
		r.ClearSelection()
	}
	if left && raw.Type == MouseUp {
		r.pressedMouse = nil
	}
}

func (r *Renderer) sendMouse(target *Node, event *MouseEvent) {
	event.Target = target
	for n := target; n != nil; n = n.parent {
		event.CurrentTarget = n
		event.LocalX, event.LocalY = event.X-n.rect.X, event.Y-n.rect.Y
		if n.mouseHandler != nil {
			n.mouseHandler(event)
		}
		if !event.PropagationStopped() && n.mouseHandlers != nil {
			if handler := n.mouseHandlers[event.Type]; handler != nil {
				handler(event)
			}
		}
		if !event.PropagationStopped() && !event.DefaultPrevented() && n.controlMouseHandler != nil {
			n.controlMouseHandler(event)
		}
		if event.PropagationStopped() {
			break
		}
	}
}

func (r *Renderer) hitTest(x, y int) *Node {
	var visit func(*Node, Rect) *Node
	visit = func(n *Node, clip Rect) *Node {
		if n.Hidden || !clip.Contains(x, y) || !n.rect.Contains(x, y) {
			return nil
		}
		// Scrollbars paint over a ScrollBox's content, so they also win hits.
		if n.scroll != nil && n.scroll.barAt(x, y) != barNone {
			return n
		}
		childClip := clip
		if n.Style.Overflow == Hidden {
			childClip = childClip.Intersect(n.rect.Inset(n.border))
		}
		order := n.paintOrder()
		for i := len(order) - 1; i >= 0; i-- {
			if hit := visit(order[i], childClip); hit != nil {
				return hit
			}
		}
		return n
	}
	return visit(r.root, r.root.rect)
}

func cloneMouse(event *MouseEvent, eventType MouseEventType) *MouseEvent {
	clone := *event
	clone.Type, clone.Target, clone.CurrentTarget = eventType, nil, nil
	clone.defaultPrevented, clone.propagationStopped = false, false
	return &clone
}

func isDescendant(node, ancestor *Node) bool {
	for n := node; n != nil; n = n.parent {
		if n == ancestor {
			return true
		}
	}
	return false
}
