package opentui

import (
	"strings"
	"unicode/utf8"
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

	// runs holds per-line style runs from SetSpans (nil for plain text).
	// Each line's runs tile it in rune offsets and start on grapheme
	// boundaries.
	runs [][]styleRun

	// rows caches the visual rows for rowsWidth; nil after SetText.
	rows      []visualRow
	rowsWidth int
}

// styleRun styles runes [start, end) of a line.
type styleRun struct {
	start, end int
	fg, bg     RGBA
	attr       Attr
	link       string
}

// visualRow is one painted row of a Text: line is the logical line index,
// text the row's content, and soft reports that the row continues the
// previous row of the same logical line.
type visualRow struct {
	line  int
	start int // rune offset of the row in its line
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
			t.rows = append(t.rows, visualRow{line: i, start: row.start, text: string(runes[row.start:row.end]), width: row.width, soft: k > 0})
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
		n.text.runs = nil
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
		n.text.drawRow(b, row, r.X, r.Y+i, 0, -1, nil)
	}
	for _, selected := range n.text.selection {
		if selected.line < 0 || selected.line >= len(rows) {
			continue
		}
		row := rows[selected.line]
		from, to := cellsToRunes(row.text, selected.start, selected.end)
		if from < to {
			colors := [2]RGBA{o.SelectionFG, o.SelectionBG}
			n.text.drawRow(b, row, r.X, r.Y+selected.line, from, to, &colors)
		}
	}
}

// drawRow paints runes [from, to) of a visual row (to < 0: to the end)
// with its style runs, at x0 (the row's first cell), y. colors, when set,
// replaces the foreground and background, as a selection highlight does;
// attributes and links are kept.
func (t *textState) drawRow(b *Buffer, row visualRow, x0, y, from, to int, colors *[2]RGBA) {
	o := &t.opts
	if t.runs == nil && from == 0 && to < 0 && colors == nil {
		b.DrawText(row.text, x0, y, o.FG, o.BG, o.Attr) // plain text: no rune slicing
		return
	}
	runes := []rune(row.text)
	if to < 0 || to > len(runes) {
		to = len(runes)
	}
	if from >= to {
		return
	}
	x := x0 + displayWidth(string(runes[:from]))
	var runs []styleRun
	if t.runs != nil && row.line < len(t.runs) {
		runs = t.runs[row.line]
	}
	if len(runs) == 0 {
		fg, bg := o.FG, o.BG
		if colors != nil {
			fg, bg = colors[0], colors[1]
		}
		b.DrawText(string(runes[from:to]), x, y, fg, bg, o.Attr)
		return
	}
	for _, run := range runs {
		// The run's part of this row, in row-relative runes.
		a, z := max(run.start-row.start, from), min(run.end-row.start, to)
		if a >= z {
			continue
		}
		fg, bg := o.FG, o.BG
		if run.fg != (RGBA{}) {
			fg = run.fg
		}
		if run.bg != (RGBA{}) {
			bg = run.bg
		}
		if colors != nil {
			fg, bg = colors[0], colors[1]
		}
		segment := string(runes[a:z])
		cx := x + displayWidth(string(runes[from:a]))
		b.DrawTextLink(segment, run.link, cx, y, fg, bg, o.Attr|run.attr)
	}
}

// cellsToRunes converts the cell range [from, to) of a row to rune
// offsets, widened to whole grapheme clusters.
func cellsToRunes(text string, from, to int) (int, int) {
	start, end, runePos, set := 0, 0, 0, false
	for _, c := range clusters(text) {
		n := utf8.RuneCountInString(c.text)
		if c.end > from && c.start < to {
			if !set {
				start, set = runePos, true
			}
			end = runePos + n
		}
		runePos += n
	}
	return start, end
}

// Span is a styled piece of text for Styled and SetSpans, the counterpart
// of upstream's TextChunk. Zero colors use the Text's FG and BG; Attr is
// added to the Text's Attr. Link makes the span an OSC 8 hyperlink when
// the terminal supports them (see Options.Hyperlinks).
type Span struct {
	Text   string
	FG, BG RGBA
	Attr   Attr
	Link   string
}

// S starts a span; chain Bold, Fg, Link and friends to style it:
//
//	opentui.Styled(opts, opentui.S("error: ").Bold().Fg(red), opentui.S(msg))
func S(text string) Span { return Span{Text: text} }

// Link returns a span that links text to url.
func Link(url, text string) Span { return Span{Text: text, Link: url} }

func (s Span) Bold() Span          { s.Attr |= AttrBold; return s }
func (s Span) Dim() Span           { s.Attr |= AttrDim; return s }
func (s Span) Italic() Span        { s.Attr |= AttrItalic; return s }
func (s Span) Underline() Span     { s.Attr |= AttrUnderline; return s }
func (s Span) Strikethrough() Span { s.Attr |= AttrStrikethrough; return s }
func (s Span) Inverse() Span       { s.Attr |= AttrInverse; return s }
func (s Span) Fg(c RGBA) Span      { s.FG = c; return s }
func (s Span) Bg(c RGBA) Span      { s.BG = c; return s }

// WithLink makes the span a hyperlink to url.
func (s Span) WithLink(url string) Span { s.Link = url; return s }

// Styled returns a Text node showing spans; spans may contain newlines.
func Styled(o TextOptions, spans ...Span) *Node {
	n := Text("", o)
	n.SetSpans(spans...)
	return n
}

// SetSpans replaces a Text node's content with styled spans. Text with a
// single style can keep using SetText, which clears the spans.
func (n *Node) SetSpans(spans ...Span) {
	t := n.text
	if t == nil {
		return
	}
	var lines []string
	var runs [][]styleRun
	var line []rune
	var lineRuns []styleRun
	flush := func() {
		lines = append(lines, string(line))
		runs = append(runs, snapRuns(line, lineRuns))
		line, lineRuns = nil, nil
	}
	for _, s := range spans {
		link := sanitizeLink(s.Link)
		for i, part := range strings.Split(s.Text, "\n") {
			if i > 0 {
				flush()
			}
			r := []rune(part)
			if len(r) > 0 {
				lineRuns = append(lineRuns, styleRun{len(line), len(line) + len(r), s.FG, s.BG, s.Attr, link})
				line = append(line, r...)
			}
		}
	}
	flush()
	t.lines, t.runs, t.rows = lines, runs, nil
}

// snapRuns moves run boundaries that fall inside a grapheme cluster to the
// cluster's end, so each cluster takes the style of its first rune and is
// never drawn in two pieces. Runs left empty are dropped.
func snapRuns(line []rune, runs []styleRun) []styleRun {
	out := runs[:0]
	prevEnd := 0
	for _, r := range runs {
		r.start = prevEnd
		r.end = inputCeil(line, r.end)
		if r.end > r.start {
			out = append(out, r)
			prevEnd = r.end
		}
	}
	return out
}

// Spans returns a Text node's content as spans: the spans from SetSpans
// (merged per line and split at newlines), or one plain span after
// SetText.
func (n *Node) Spans() []Span {
	t := n.text
	if t == nil {
		return nil
	}
	if t.runs == nil {
		return []Span{{Text: strings.Join(t.lines, "\n")}}
	}
	var out []Span
	for i, line := range t.lines {
		runes := []rune(line)
		if i > 0 {
			out = append(out, Span{Text: "\n"})
		}
		for _, r := range t.runs[i] {
			out = append(out, Span{Text: string(runes[r.start:r.end]), FG: r.fg, BG: r.bg, Attr: r.attr, Link: r.link})
		}
	}
	return out
}
