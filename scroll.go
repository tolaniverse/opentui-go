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

	// Scrollbar drag in progress: dragAxis is barNone, barX or barY, and
	// dragGrab is where on the thumb the pointer took hold.
	dragAxis, dragGrab int
}

const (
	barNone = iota
	barX
	barY
)

// bar describes one painted scrollbar in screen cells. Along is the
// coordinate on the bar's axis; cross is the fixed row or column.
type bar struct {
	trackStart, trackLen int
	thumbStart, thumbLen int
	cross                int
}

// bar returns the geometry of the scrollbar on axis, or false when it is
// hidden or has nothing to scroll.
func (s *scrollState) bar(axis int) (bar, bool) {
	if s.hideBars || s.viewport.Empty() {
		return bar{}, false
	}
	var length, content, offset, maxOffset, start, cross int
	switch axis {
	case barY:
		length, content, offset, maxOffset = s.viewport.H, s.contentH, s.y, s.maxY
		start, cross = s.viewport.Y, s.viewport.X+s.viewport.W-1
	case barX:
		length, content, offset, maxOffset = s.viewport.W, s.contentW, s.x, s.maxX
		start, cross = s.viewport.X, s.viewport.Y+s.viewport.H-1
	default:
		return bar{}, false
	}
	if maxOffset <= 0 {
		return bar{}, false
	}
	size := min(max(length*length/max(content, 1), 1), length)
	return bar{
		trackStart: start, trackLen: length,
		thumbStart: start + (length-size)*offset/maxOffset, thumbLen: size,
		cross: cross,
	}, true
}

// barAt reports which scrollbar, if any, covers screen cell x, y. The
// vertical bar wins the shared corner cell.
func (s *scrollState) barAt(x, y int) int {
	if b, ok := s.bar(barY); ok && x == b.cross && y >= b.trackStart && y < b.trackStart+b.trackLen {
		return barY
	}
	if b, ok := s.bar(barX); ok && y == b.cross && x >= b.trackStart && x < b.trackStart+b.trackLen {
		return barX
	}
	return barNone
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
	// Box padding belongs to the scrolled content, as upstream ScrollBox
	// forwards root padding to its content node. The content is absolutely
	// positioned, and Yoga places absolute children at the padding edge, so
	// padding left on the box would be ignored by the content yet still
	// shrink the viewport.
	p := o.BoxOptions.Style.Padding
	contentStyle.Padding = Edges{
		Top: contentStyle.Padding.Top + p.Top, Right: contentStyle.Padding.Right + p.Right,
		Bottom: contentStyle.Padding.Bottom + p.Bottom, Left: contentStyle.Padding.Left + p.Left,
	}
	o.BoxOptions.Style.Padding = Edges{}
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
	s := n.scroll
	switch event.Type {
	case MouseDown:
		if event.Button == MouseLeft && n.pressScrollbar(event.X, event.Y) {
			event.StopPropagation()
		}
		return
	case MouseDrag:
		if s.dragAxis != barNone {
			n.dragScrollbar(event.X, event.Y)
			event.StopPropagation()
		}
		return
	case MouseUp, MouseDragEnd:
		s.dragAxis = barNone
		return
	case MouseScroll:
	default:
		return
	}
	if event.Scroll == nil {
		return
	}
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

// pressScrollbar handles a left press. On the thumb it starts a drag; on
// the track it pages toward the pointer, then drags from there.
func (n *Node) pressScrollbar(x, y int) bool {
	s := n.scroll
	axis := s.barAt(x, y)
	if axis == barNone {
		return false
	}
	b, _ := s.bar(axis)
	pos := y
	if axis == barX {
		pos = x
	}
	if pos < b.thumbStart || pos >= b.thumbStart+b.thumbLen {
		page := max(b.trackLen-1, 1)
		if pos < b.thumbStart {
			page = -page
		}
		if axis == barY {
			n.ScrollBy(0, page)
		} else {
			n.ScrollBy(page, 0)
		}
		b, _ = s.bar(axis)
	}
	s.dragAxis = axis
	s.dragGrab = min(max(pos-b.thumbStart, 0), b.thumbLen-1)
	return true
}

// dragScrollbar keeps the grabbed point of the thumb under the pointer.
func (n *Node) dragScrollbar(x, y int) {
	s := n.scroll
	b, ok := s.bar(s.dragAxis)
	if !ok {
		return
	}
	pos, maxOffset := y, s.maxY
	if s.dragAxis == barX {
		pos, maxOffset = x, s.maxX
	}
	travel := b.trackLen - b.thumbLen
	if travel <= 0 {
		return
	}
	thumb := min(max(pos-s.dragGrab-b.trackStart, 0), travel)
	offset := (thumb*maxOffset + travel/2) / travel // rounded
	if s.dragAxis == barY {
		n.ScrollTo(s.x, offset)
	} else {
		n.ScrollTo(offset, s.y)
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
	if s == nil {
		return
	}
	track := RGB(69, 71, 90)
	thumb := RGB(137, 180, 250)
	if g, ok := s.bar(barY); ok {
		b.FillRect(g.cross, g.trackStart, 1, g.trackLen, track)
		b.FillRect(g.cross, g.thumbStart, 1, g.thumbLen, thumb)
	}
	if g, ok := s.bar(barX); ok {
		b.FillRect(g.trackStart, g.cross, g.trackLen, 1, track)
		b.FillRect(g.thumbStart, g.cross, g.thumbLen, 1, thumb)
	}
}
