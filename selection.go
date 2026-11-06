package opentui

import (
	"sort"
	"strings"
	"time"
	"unicode"
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
	fx, fy := r.selection.focusX, r.selection.focusY
	if ay > fy || (ay == fy && ax > fx) {
		ax, fx, ay, fy = fx, ax, fy, ay
	}
	type segment struct {
		y, x int
		text string
	}
	var segments []segment
	for _, n := range texts {
		base := n.rect.Inset(n.Style.Padding)
		for lineIndex, line := range n.text.lines {
			y := base.Y + lineIndex
			if y < ay || y > fy {
				continue
			}
			runes := []rune(line)
			start, end := 0, len(runes)
			if y == ay {
				start = ax - base.X
			}
			if y == fy {
				end = fx - base.X + 1
			}
			start, end = min(max(start, 0), len(runes)), min(max(end, 0), len(runes))
			if r.selection.Behavior == SelectLine {
				start, end = 0, len(runes)
			} else if r.selection.Behavior == SelectWord && ay == fy && y == ay {
				point := min(max(r.selection.anchorRelX-n.Style.Padding.Left, 0), max(len(runes)-1, 0))
				start, end = wordBounds(runes, point)
			} else if r.selection.Behavior == SelectCell && ax == fx && ay == fy {
				start, end = 0, 0
			}
			if start >= end {
				continue
			}
			n.text.selection = append(n.text.selection, textSelection{line: lineIndex, start: start, end: end})
			segments = append(segments, segment{y: y, x: base.X + start, text: string(runes[start:end])})
		}
	}
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].y != segments[j].y {
			return segments[i].y < segments[j].y
		}
		return segments[i].x < segments[j].x
	})
	var out strings.Builder
	lastY := -1
	for _, segment := range segments {
		if lastY != -1 && segment.y != lastY {
			out.WriteByte('\n')
		}
		out.WriteString(segment.text)
		lastY = segment.y
	}
	r.selection.text = out.String()
}

func wordBounds(runes []rune, point int) (start, end int) {
	if len(runes) == 0 {
		return 0, 0
	}
	point = min(max(point, 0), len(runes)-1)
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	class := isWord(runes[point])
	start, end = point, point+1
	for start > 0 && isWord(runes[start-1]) == class {
		start--
	}
	for end < len(runes) && isWord(runes[end]) == class {
		end++
	}
	return start, end
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
