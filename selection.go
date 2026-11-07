package opentui

import (
	"sort"
	"strings"
	"time"
)

// SelectionBehavior controls how selection endpoints expand.
type SelectionBehavior string

const (
	SelectCell SelectionBehavior = "cell"
	SelectWord SelectionBehavior = "word"
	SelectLine SelectionBehavior = "line"
)

// Selection is the renderer's current text selection.
type Selection struct {
	anchorNode             *Node
	anchorRelX, anchorRelY int
	focusX, focusY         int
	Behavior               SelectionBehavior
	Dragging               bool
	text                   string
}

// Anchor returns the current screen position of the anchored node cell.
func (s *Selection) Anchor() (x, y int) {
	if s == nil || s.anchorNode == nil {
		return 0, 0
	}
	return s.anchorNode.rect.X + s.anchorRelX, s.anchorNode.rect.Y + s.anchorRelY
}

// Focus returns the moving endpoint in screen cells.
func (s *Selection) Focus() (x, y int) {
	if s == nil {
		return 0, 0
	}
	return s.focusX, s.focusY
}

// Text returns selected text in screen reading order.
func (s *Selection) Text() string {
	if s == nil {
		return ""
	}
	return s.text
}

type textSelection struct{ line, start, end int }

// Selection returns the active or completed selection, or nil.
func (r *Renderer) Selection() *Selection { return r.selection }

// SelectedText returns the current selection's text.
func (r *Renderer) SelectedText() string {
	if r.selection == nil {
		return ""
	}
	return r.selection.Text()
}

// SetSelectionHandler installs a callback fired when a drag selection ends.
func (r *Renderer) SetSelectionHandler(fn func(*Selection)) { r.selectionHandler = fn }

// ClearSelection removes selection state and highlighting.
func (r *Renderer) ClearSelection() {
	r.selection = nil
	r.updateTextSelections()
}

func (r *Renderer) startSelection(node *Node, x, y int) {
	behavior := r.nextSelectionBehavior(node, x, y)
	r.selection = &Selection{
		anchorNode: node, anchorRelX: x - node.rect.X, anchorRelY: y - node.rect.Y,
		focusX: x, focusY: y, Behavior: behavior, Dragging: true,
	}
	r.updateTextSelections()
}

func (r *Renderer) updateSelection(x, y int) {
	if r.selection == nil {
		return
	}
	r.selection.focusX, r.selection.focusY = x, y
	r.updateTextSelections()
}

func (r *Renderer) finishSelection() {
	if r.selection == nil {
		return
	}
	r.selection.Dragging = false
	r.updateTextSelections()
	if r.selectionHandler != nil {
		r.selectionHandler(r.selection)
	}
}

func (r *Renderer) nextSelectionBehavior(node *Node, x, y int) SelectionBehavior {
	now := time.Now()
	continued := node == r.lastClickNode && now.Sub(r.lastClickTime) <= 500*time.Millisecond &&
		max(absInt(x-r.lastClickX), absInt(y-r.lastClickY)) <= 1
	if continued {
		r.lastClickCount = min(r.lastClickCount+1, 3)
	} else {
		r.lastClickCount = 1
	}
	r.lastClickNode, r.lastClickX, r.lastClickY, r.lastClickTime = node, x, y, now
	if r.lastClickCount == 2 {
		return SelectWord
	}
	if r.lastClickCount == 3 {
		return SelectLine
	}
	return SelectCell
}

func (r *Renderer) updateTextSelections() {
	var texts []*Node
	var walk func(*Node)
	walk = func(n *Node) {
		if n.text != nil {
			n.text.selection = nil
			if n.Selectable && !n.Hidden {
				texts = append(texts, n)
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(r.root)
	if r.selection == nil {
		return
	}

	ax, ay := r.selection.Anchor()
	fx, fy := r.selectionFocus()
	if ay > fy || (ay == fy && ax > fx) {
		ax, fx, ay, fy = fx, ax, fy, ay
	}
	type segment struct {
		y, x int
		text string
		node *Node
		row  int
	}
	var segments []segment
	for _, n := range texts {
		base := n.rect.Inset(n.Style.Padding)
		for lineIndex, row := range n.text.visual(base.W) {
			line := row.text
			y := base.Y + lineIndex
			if y < ay || y > fy {
				continue
			}
			width := displayWidth(line)
			start, end := 0, width
			if y == ay {
				start = ax - base.X
			}
			if y == fy {
				end = fx - base.X + 1
			}
			start, end = min(max(start, 0), width), min(max(end, 0), width)
			if r.selection.Behavior == SelectLine {
				start, end = 0, width
			} else if r.selection.Behavior == SelectWord && ay == fy && y == ay {
				point := r.selection.anchorRelX - n.Style.Padding.Left
				start, end = wordColumns(line, point)
			} else if r.selection.Behavior == SelectCell && ax == fx && ay == fy {
				start, end = 0, 0
			}
			if start >= end {
				continue
			}
			text, from, to := selectedColumns(line, start, end)
			if text == "" {
				continue
			}
			n.text.selection = append(n.text.selection, textSelection{line: lineIndex, start: from, end: to})
			segments = append(segments, segment{y: y, x: base.X + from, text: text, node: n, row: lineIndex})
		}
	}
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].y != segments[j].y {
			return segments[i].y < segments[j].y
		}
		return segments[i].x < segments[j].x
	})
	var out strings.Builder
	for i, segment := range segments {
		// Rows scrolled above the screen have negative y, so the first row
		// is tracked by index rather than with a sentinel row number.
		if i > 0 && segment.y != segments[i-1].y && !softContinuation(segments[i-1].node, segments[i-1].row, segment.node, segment.row) {
			out.WriteByte('\n')
		}
		out.WriteString(segment.text)
	}
	r.selection.text = out.String()
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// selectionScrollBox returns the nearest ScrollBox containing the
// selection anchor, or nil.
func (r *Renderer) selectionScrollBox() *Node {
	if r.selection == nil || r.selection.anchorNode == nil {
		return nil
	}
	for n := r.selection.anchorNode.parent; n != nil; n = n.parent {
		if n.scroll != nil {
			return n
		}
	}
	return nil
}

// selectionFocus is the focus point used for highlighting. Inside a
// ScrollBox it is clamped to the visible viewport, so a pointer dragged
// past the edge selects up to the last visible cell instead of selecting
// hidden content at the pointer's screen row.
func (r *Renderer) selectionFocus() (x, y int) {
	x, y = r.selection.focusX, r.selection.focusY
	box := r.selectionScrollBox()
	if box == nil || box.scroll.viewport.Empty() {
		return x, y
	}
	vp := box.scroll.viewport
	switch {
	case y < vp.Y:
		return vp.X, vp.Y
	case y >= vp.Y+vp.H:
		return vp.X + vp.W - 1, vp.Y + vp.H - 1
	}
	return min(max(x, vp.X), vp.X+vp.W-1), y
}

// autoScrollInterval is how often a selection drag past a ScrollBox edge
// scrolls it, while the pointer stays outside.
const autoScrollInterval = 50 * time.Millisecond

// autoScrollDelta is how far the selection's ScrollBox should move for the
// current pointer position: one line at the edge, faster further out.
func (r *Renderer) autoScrollDelta() (box *Node, dx, dy int) {
	if r.selection == nil || !r.selection.Dragging {
		return nil, 0, 0
	}
	box = r.selectionScrollBox()
	if box == nil {
		return nil, 0, 0
	}
	vp := box.scroll.viewport
	speed := func(distance int) int { return min(1+distance/2, 5) }
	px, py := r.selection.focusX, r.selection.focusY
	if py < vp.Y {
		dy = -speed(vp.Y - py)
	} else if py >= vp.Y+vp.H {
		dy = speed(py - (vp.Y + vp.H - 1))
	}
	if px < vp.X {
		dx = -speed(vp.X - px)
	} else if px >= vp.X+vp.W {
		dx = speed(px - (vp.X + vp.W - 1))
	}
	return box, dx, dy
}

// autoScrollTick scrolls once and reports whether anything moved.
func (r *Renderer) autoScrollTick() bool {
	box, dx, dy := r.autoScrollDelta()
	return box != nil && (dx != 0 || dy != 0) && box.ScrollBy(dx, dy)
}

// scheduleAutoScroll keeps scrolling on a timer while a selection drag is
// held past a ScrollBox edge. It must run on the Run goroutine; the timer
// only queues work back onto it through Update.
func (r *Renderer) scheduleAutoScroll() {
	if r.autoScrolling || r.updates == nil {
		return
	}
	if box, dx, dy := r.autoScrollDelta(); box == nil || (dx == 0 && dy == 0) {
		return
	}
	r.autoScrolling = true
	time.AfterFunc(autoScrollInterval, func() {
		r.Update(func() {
			r.autoScrolling = false
			if r.autoScrollTick() {
				r.scheduleAutoScroll()
			}
		})
	})
}

// drawFrame lays out the tree, refreshes the selection against the new
// layout (content may have scrolled under it), then paints.
func (r *Renderer) drawFrame(b *Buffer) {
	ComputeLayout(r.root, b.Width(), b.Height())
	if r.selection != nil {
		r.updateTextSelections()
	}
	Paint(r.root, b)
}

// softContinuation reports whether row b of node nb directly continues row a
// of node na after a soft wrap, so selected text joins them without "\n".
func softContinuation(na *Node, a int, nb *Node, b int) bool {
	if na != nb || b != a+1 || nb.text == nil {
		return false
	}
	rows := nb.text.rows
	return b < len(rows) && rows[b].soft
}
