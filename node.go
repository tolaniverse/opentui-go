package opentui

import (
	"slices"
)

// Node is one element of the UI tree: a box, some text, or a plain
// layout container. Port of the core of upstream `Renderable`
// (Renderable.ts): parent/child links, layout style, z-order, visibility.
//
// A Node is not safe for concurrent use. Change the tree only from the
// Run goroutine — inside the draw callback or a Renderer.Update func.
type Node struct {
	ID     string
	Style  Style
	ZIndex int  // higher paints later (on top); ties keep insertion order
	Hidden bool // hidden nodes take no space and do not paint
	// Focusable includes this node in renderer focus traversal.
	Focusable bool
	// Selectable allows the renderer to begin text selection on this node.
	Selectable bool

	parent   *Node
	children []*Node

	border              Edges                    // set by Box
	measure             MeasureFunc              // leaves (Text): content size
	paint               func(b *Buffer, n *Node) // draws this node only
	paintAfter          func(b *Buffer, n *Node) // draws over children
	text                *textState               // set by Text
	input               *inputState              // set by Input
	keyHandler          func(*KeyEvent)          // control default, set by Input
	pasteHandler        func(*PasteEvent)
	userKeyHandler      func(*KeyEvent)
	userPasteHandler    func(*PasteEvent)
	mouseHandler        func(*MouseEvent)
	mouseHandlers       map[MouseEventType]func(*MouseEvent)
	controlMouseHandler func(*MouseEvent)
	scroll              *scrollState
	onFocus             func()
	onBlur              func()
	focusHandler        func()
	blurHandler         func()
	focused             bool

	lay  layoutState // Yoga working data, fractional cells
	rect Rect        // final layout, rounded to whole cells
}

// NewNode returns a layout-only container (draws nothing itself).
func NewNode(style Style, children ...*Node) *Node {
	n := &Node{Style: style}
	n.Add(children...)
	return n
}

// Add appends children. A child that already has a parent is moved.
// Adding a node to itself or to one of its descendants panics, because
// the tree would become a loop. So does adding children to a leaf that
// measures its own content (Text), as in Yoga.
func (n *Node) Add(children ...*Node) *Node {
	if n.scroll != nil && n.scroll.content != nil {
		n.scroll.content.Add(children...)
		return n
	}
	for _, c := range children {
		if c != nil && n.measure != nil {
			panic("opentui: a measured leaf (e.g. Text) cannot have children")
		}
		if c == nil {
			continue
		}
		for p := n; p != nil; p = p.parent {
			if p == c {
				panic("opentui: Add would create a cycle")
			}
		}
		if c.parent != nil {
			c.parent.Remove(c)
		}
		c.parent = n
		n.children = append(n.children, c)
	}
	return n
}

// Remove detaches child from n. It reports whether child was found.
func (n *Node) Remove(child *Node) bool {
	if n.scroll != nil && n.scroll.content != nil && child != n.scroll.content {
		return n.scroll.content.Remove(child)
	}
	i := slices.Index(n.children, child)
	if i < 0 {
		return false
	}
	n.children = slices.Delete(n.children, i, i+1)
	child.parent = nil
	return true
}

// Parent returns the node's parent, or nil for a root.
func (n *Node) Parent() *Node { return n.parent }

// Children returns a copy of the child list.
func (n *Node) Children() []*Node {
	if n.scroll != nil && n.scroll.content != nil {
		return n.scroll.content.Children()
	}
	return slices.Clone(n.children)
}

// Rect is where the last layout put this node, in screen cells.
func (n *Node) Rect() Rect { return n.rect }

// Focused reports whether the renderer currently directs keyboard input to n.
func (n *Node) Focused() bool { return n.focused }

// SetKeyHandler installs the node's focused key handler. Passing nil removes it.
func (n *Node) SetKeyHandler(fn func(*KeyEvent)) { n.userKeyHandler = fn }

// SetPasteHandler installs the node's focused paste handler.
func (n *Node) SetPasteHandler(fn func(*PasteEvent)) { n.userPasteHandler = fn }

// SetMouseHandler installs a handler for every mouse event delivered to n.
func (n *Node) SetMouseHandler(fn func(*MouseEvent)) { n.mouseHandler = fn }

// SetMouseEventHandler installs a handler for one mouse event type.
func (n *Node) SetMouseEventHandler(eventType MouseEventType, fn func(*MouseEvent)) {
	if fn == nil {
		delete(n.mouseHandlers, eventType)
		return
	}
	if n.mouseHandlers == nil {
		n.mouseHandlers = make(map[MouseEventType]func(*MouseEvent))
	}
	n.mouseHandlers[eventType] = fn
}

// SetFocusHandler installs a callback invoked when n receives focus.
func (n *Node) SetFocusHandler(fn func()) { n.focusHandler = fn }

// SetBlurHandler installs a callback invoked when n loses focus.
func (n *Node) SetBlurHandler(fn func()) { n.blurHandler = fn }

// Find returns the first node (depth-first) with the given ID, or nil.
func (n *Node) Find(id string) *Node {
	if n.ID == id {
		return n
	}
	for _, c := range n.children {
		if f := c.Find(id); f != nil {
			return f
		}
	}
	return nil
}

// Draw lays out root to fill b, then paints the tree into b.
func Draw(root *Node, b *Buffer) {
	ComputeLayout(root, b.Width(), b.Height())
	Paint(root, b)
}

// Paint draws an already laid-out tree: each node paints itself, then its
// children in z-order. Children of an Overflow: Hidden node are clipped to
// the area inside its border (upstream scene.zig does the same).
func Paint(n *Node, b *Buffer) {
	if n.Hidden {
		return
	}
	if n.paint != nil {
		n.paint(b, n)
	}
	if len(n.children) == 0 {
		return
	}
	clipped := n.Style.Overflow == Hidden
	if clipped {
		b.PushClip(n.rect.Inset(n.border))
		defer b.PopClip()
	}
	for _, c := range n.paintOrder() {
		Paint(c, b)
	}
	if n.paintAfter != nil {
		n.paintAfter(b, n)
	}
}

// paintOrder sorts children by ZIndex, then insertion order.
func (n *Node) paintOrder() []*Node {
	sorted := true
	for i := 1; i < len(n.children); i++ {
		if n.children[i-1].ZIndex > n.children[i].ZIndex {
			sorted = false
			break
		}
	}
	if sorted {
		return n.children // common case: no allocation
	}
	order := slices.Clone(n.children)
	slices.SortStableFunc(order, func(a, b *Node) int {
		if a.ZIndex < b.ZIndex {
			return -1
		}
		if a.ZIndex > b.ZIndex {
			return 1
		}
		return 0 // stable sort preserves insertion order
	})
	return order
}
