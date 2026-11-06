package opentui

import (
	"strings"
	"unicode/utf8"
)

// TextOptions configures Text.
type TextOptions struct {
	ID     string
	Style  Style
	ZIndex int
	FG     RGBA // zero value = white
	BG     RGBA // Transparent (zero) = show what is underneath
	Attr   Attr
}

type textState struct {
	lines []string
	opts  TextOptions
}

// Text returns a leaf node that shows content. Lines split on "\n".
// Its natural size is the longest line × the number of lines.
//
// v0.2.0 is a simplified stand-in for upstream TextRenderable: no
// wrapping, no styled spans, one cell per rune.
func Text(content string, o TextOptions) *Node {
	n := NewNode(o.Style)
	n.ID, n.ZIndex = o.ID, o.ZIndex
	if o.FG == (RGBA{}) {
		o.FG = White
	}
	n.text = &textState{opts: o}
	n.SetText(content)
	// measure is Yoga's "measure function": how a leaf tells layout how
	// big its content is, within the limits it is given.
	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		cw := 0
		for _, l := range n.text.lines {
			cw = max(cw, utf8.RuneCountInString(l))
		}
		return fit(float64(cw), w, wm), fit(float64(len(n.text.lines)), h, hm)
	}
	n.paint = paintText
	return n
}

// fit applies a measure constraint to a content size.
func fit(content, limit float64, m MeasureMode) float64 {
	switch m {
	case MeasureExactly:
		return limit
	case MeasureAtMost:
		return min(content, limit)
	}
	return content
}

// SetText replaces the content of a Text node. On other nodes it does nothing.
func (n *Node) SetText(content string) {
	if n.text != nil {
		n.text.lines = strings.Split(content, "\n")
	}
}

// TextContent returns the content of a Text node ("" for other nodes).
func (n *Node) TextContent() string {
	if n.text == nil {
		return ""
	}
	return strings.Join(n.text.lines, "\n")
}

func paintText(b *Buffer, n *Node) {
	r := n.rect.Inset(n.Style.Padding)
	if r.Empty() {
		return
	}
	o := &n.text.opts
	b.PushClip(r) // text never spills outside its own box
	defer b.PopClip()
	if o.BG.A > 0 {
		b.FillRect(r.X, r.Y, r.W, r.H, o.BG)
	}
	for i, line := range n.text.lines {
		b.DrawText(line, r.X, r.Y+i, o.FG, o.BG, o.Attr)
	}
}
