package opentui

// ScrollBoxOptions configures a clipped, focusable scrolling box. Children
// are laid out inside an internal content node and moved beneath the viewport.
type ScrollBoxOptions struct {
	BoxOptions
	ContentStyle     Style
	ScrollX, ScrollY bool
	StickyEnd        bool
	HideScrollbars   bool
	WheelStep        int
	OnScroll         func(x, y int)
}

type scrollState struct {
	content            *Node
	x, y               int
	maxX, maxY         int
	contentW, contentH int
	viewport           Rect
	scrollX, scrollY   bool
	stickyEnd          bool
	hideBars           bool
	wheelStep          int
	onScroll           func(int, int)
}

// ScrollBox returns a box with clipped scrollable content. Vertical scrolling
// is enabled when neither axis is specified.
func ScrollBox(o ScrollBoxOptions, children ...*Node) *Node {
	if !o.ScrollX && !o.ScrollY {
		o.ScrollY = true
	}
	if o.WheelStep <= 0 {
		o.WheelStep = 3
	}
	contentStyle := o.ContentStyle
	contentStyle.Position = Absolute
	contentStyle.Left, contentStyle.Top = Cells(0), Cells(0)
	if !o.ScrollX && contentStyle.Width.IsAuto() {
		contentStyle.Width = Pct(100)
	}
	content := NewNode(contentStyle, children...)
	o.BoxOptions.Style.Overflow = Hidden
	n := Box(o.BoxOptions, content)
	n.Focusable = true
	n.scroll = &scrollState{
		content: content, scrollX: o.ScrollX, scrollY: o.ScrollY,
		stickyEnd: o.StickyEnd, hideBars: o.HideScrollbars,
		wheelStep: o.WheelStep, onScroll: o.OnScroll,
	}
	n.controlMouseHandler = n.handleScrollMouse
	n.keyHandler = n.handleScrollKey
	n.paintAfter = paintScrollbars
	return n
}

// ScrollBy moves a ScrollBox by delta cells and reports whether it moved.
func (n *Node) ScrollBy(dx, dy int) bool {
	if n.scroll == nil {
		return false
	}
	return n.setScroll(n.scroll.x+dx, n.scroll.y+dy)
}

// ScrollTo moves a ScrollBox to an absolute cell offset.
func (n *Node) ScrollTo(x, y int) bool {
	if n.scroll == nil {
		return false
	}
	return n.setScroll(x, y)
}

// ScrollOffset returns a ScrollBox's current x and y offsets.
func (n *Node) ScrollOffset() (x, y int) {
	if n.scroll == nil {
		return 0, 0
	}
	return n.scroll.x, n.scroll.y
}

// ScrollSize returns a ScrollBox's laid-out content dimensions.
func (n *Node) ScrollSize() (width, height int) {
	if n.scroll == nil {
		return 0, 0
	}
	return n.scroll.contentW, n.scroll.contentH
}

// ViewportRect returns the visible content rectangle of a ScrollBox.
func (n *Node) ViewportRect() Rect {
	if n.scroll == nil {
		return Rect{}
	}
	return n.scroll.viewport
}

func (n *Node) setScroll(x, y int) bool {
	s := n.scroll
	if !s.scrollX {
		x = 0
	}
	if !s.scrollY {
		y = 0
	}
	x, y = min(max(x, 0), s.maxX), min(max(y, 0), s.maxY)
	if x == s.x && y == s.y {
		return false
	}
	s.x, s.y = x, y
	if s.onScroll != nil {
		s.onScroll(x, y)
	}
	return true
}

func (n *Node) handleScrollMouse(event *MouseEvent) {
	if event.Type != MouseScroll || event.Scroll == nil {
		return
	}
	s := n.scroll
	delta := event.Scroll.Delta * s.wheelStep
	moved := false
	switch event.Scroll.Direction {
	case ScrollUp:
		moved = n.ScrollBy(0, -delta)
	case ScrollDown:
		moved = n.ScrollBy(0, delta)
	case ScrollLeft:
		moved = n.ScrollBy(-delta, 0)
	case ScrollRight:
		moved = n.ScrollBy(delta, 0)
	}
	if moved {
		event.PreventDefault()
		event.StopPropagation()
	}
}

func (n *Node) handleScrollKey(key *KeyEvent) {
	if key.Type == KeyRelease {
		return
	}
	s := n.scroll
	moved := false
	switch key.Name {
	case "up":
		moved = n.ScrollBy(0, -1)
	case "down":
		moved = n.ScrollBy(0, 1)
	case "left":
		moved = n.ScrollBy(-1, 0)
	case "right":
		moved = n.ScrollBy(1, 0)
	case "pageup":
		moved = n.ScrollBy(0, -max(s.viewport.H-1, 1))
	case "pagedown":
		moved = n.ScrollBy(0, max(s.viewport.H-1, 1))
	case "home":
		moved = n.ScrollTo(0, 0)
	case "end":
		moved = n.ScrollTo(s.maxX, s.maxY)
	}
	if moved {
		key.PreventDefault()
	}
}

func prepareScrollTree(n *Node) {
	if n.Hidden {
		return
	}
	if n.scroll != nil {
		s := n.scroll
		wasEnd := s.y >= s.maxY
		s.viewport = n.rect.Inset(n.border).Inset(n.Style.Padding)
		s.contentW, s.contentH = s.content.rect.W, s.content.rect.H
		for _, child := range s.content.children {
			s.contentW = max(s.contentW, child.rect.X+child.rect.W-s.content.rect.X)
			s.contentH = max(s.contentH, child.rect.Y+child.rect.H-s.content.rect.Y)
		}
		s.maxX = max(s.contentW-s.viewport.W, 0)
		s.maxY = max(s.contentH-s.viewport.H, 0)
		if s.stickyEnd && wasEnd {
			s.y = s.maxY
		}
		s.x, s.y = min(s.x, s.maxX), min(s.y, s.maxY)
		translateNode(s.content, -s.x, -s.y)
	}
	for _, child := range n.children {
		prepareScrollTree(child)
	}
}

func translateNode(n *Node, dx, dy int) {
	n.rect.X += dx
	n.rect.Y += dy
	for _, child := range n.children {
		translateNode(child, dx, dy)
	}
}

func paintScrollbars(b *Buffer, n *Node) {
	s := n.scroll
	if s == nil || s.hideBars || s.viewport.Empty() {
		return
	}
	track := RGB(69, 71, 90)
	thumb := RGB(137, 180, 250)
	if s.maxY > 0 {
		x := s.viewport.X + s.viewport.W - 1
		b.FillRect(x, s.viewport.Y, 1, s.viewport.H, track)
		size := max(s.viewport.H*s.viewport.H/max(s.contentH, 1), 1)
		start := s.viewport.Y
		if s.maxY > 0 {
			start += (s.viewport.H - size) * s.y / s.maxY
		}
		b.FillRect(x, start, 1, size, thumb)
	}
	if s.maxX > 0 {
		y := s.viewport.Y + s.viewport.H - 1
		b.FillRect(s.viewport.X, y, s.viewport.W, 1, track)
		size := max(s.viewport.W*s.viewport.W/max(s.contentW, 1), 1)
		start := s.viewport.X
		if s.maxX > 0 {
			start += (s.viewport.W - size) * s.x / s.maxX
		}
		b.FillRect(start, y, size, 1, thumb)
	}
}
