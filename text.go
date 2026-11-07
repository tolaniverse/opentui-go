package opentui

import (
	"strings"
)

// TextOptions configures Text.
type TextOptions struct {
	ID          string
	Style       Style
	ZIndex      int
	FG          RGBA // zero value = white
	BG          RGBA // Transparent (zero) = show what is underneath
	Attr        Attr
	NoSelect    bool
	SelectionFG RGBA
	SelectionBG RGBA
	// Wrap soft-wraps lines to the node's width. The zero value, WrapNone,
	// keeps one row per line (upstream defaults to word wrapping).
	Wrap WrapMode
}

type textState struct {
	lines     []string
	opts      TextOptions
	selection []textSelection // indexes refer to visual rows

	// rows caches the visual rows for rowsWidth; nil after SetText.
	rows      []visualRow
	rowsWidth int
}

// visualRow is one painted row of a Text: line is the logical line index,
// text the row's content, and soft reports that the row continues the
// previous row of the same logical line.
type visualRow struct {
	line  int
	text  string
	width int
	soft  bool
}

// visual returns the rows of the text at width cells (ignored for
// WrapNone), recomputing only when the width or the content changed.
func (t *textState) visual(width int) []visualRow {
	if t.opts.Wrap == WrapNone {
		width = -1
	}
	if t.rows != nil && t.rowsWidth == width {
		return t.rows
	}
	t.rows, t.rowsWidth = t.rows[:0], width
	for i, line := range t.lines {
		if width < 0 {
			t.rows = append(t.rows, visualRow{line: i, text: line, width: displayWidth(line)})
			continue
		}
		runes := []rune(line)
		for k, row := range wrapLine(runes, width, t.opts.Wrap) {
			t.rows = append(t.rows, visualRow{line: i, text: string(runes[row.start:row.end]), width: row.width, soft: k > 0})
		}
	}
	return t.rows
}

// Text returns a leaf node that shows content. Lines split on "\n".
// Its natural size is the longest line × the number of lines.
//
// It is a simplified stand-in for upstream TextRenderable: plain text only,
// no styled spans. Set TextOptions.Wrap to soft-wrap to the node's width.
func Text(content string, o TextOptions) *Node {
	n := NewNode(o.Style)
	n.ID, n.ZIndex = o.ID, o.ZIndex
	if o.FG == (RGBA{}) {
		o.FG = White
	}
	if o.SelectionFG == (RGBA{}) {
		o.SelectionFG = RGB(30, 30, 46)
	}
	if o.SelectionBG == (RGBA{}) {
		o.SelectionBG = RGB(137, 180, 250)
	}
	n.text = &textState{opts: o}
	n.Selectable = !o.NoSelect
	n.SetText(content)
	// measure is Yoga's "measure function": how a leaf tells layout how
	// big its content is, within the limits it is given.
	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		// Wrapping text is as tall as its rows at the width it is offered
		// (Yoga asks again with each candidate width).
		width := -1
		if n.text.opts.Wrap != WrapNone && wm != MeasureUndefined {
			width = max(int(w), 1)
		}
		if width < 0 {
			width = 1 << 30 // no limit: wrapLine returns whole lines
		}
		rows := n.text.visual(width)
		cw := 0
		for _, row := range rows {
			cw = max(cw, row.width)
		}
		return fit(float64(cw), w, wm), fit(float64(len(rows)), h, hm)
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
		n.text.rows = nil
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
	rows := n.text.visual(r.W)
	for i, row := range rows {
		b.DrawText(row.text, r.X, r.Y+i, o.FG, o.BG, o.Attr)
	}
	for _, selected := range n.text.selection {
		if selected.line < 0 || selected.line >= len(rows) {
			continue
		}
		line := rows[selected.line].text
		selectedText, start, _ := selectedColumns(line, selected.start, selected.end)
		if selectedText != "" {
			b.DrawText(selectedText, r.X+start, r.Y+selected.line, o.SelectionFG, o.SelectionBG, o.Attr)
		}
	}
}
