package opentui

import "strings"

// TabSelectOptions configures a TabSelect. Zero colors use upstream's
// defaults.
type TabSelectOptions struct {
	ID     string
	Style  Style
	ZIndex int

	Options       []SelectOption
	SelectedIndex int
	TabWidth      int // cells per tab; zero uses 20

	BG, FG                   RGBA // unfocused; BG zero is transparent
	FocusedBG, FocusedFG     RGBA // zero FocusedBG follows a set BG, else #1a1a1a; zero FocusedFG follows FG
	SelectedBG, SelectedFG   RGBA
	SelectedDescriptionColor RGBA

	// HideDescription, HideUnderline and HideScrollArrows turn off what
	// upstream's showDescription, showUnderline and showScrollArrows
	// (default true) show.
	HideDescription  bool
	HideUnderline    bool
	HideScrollArrows bool
	WrapSelection    bool // moving past either end wraps around

	// OnChange is called after the selection moves (upstream
	// selectionChanged). Unlike Select, a move that stays put at an end
	// does not call it, as upstream TabSelect.
	OnChange func(index int, option SelectOption)
	// OnSelect is called on Enter (upstream itemSelected) and when the
	// already-selected tab is clicked.
	OnSelect func(index int, option SelectOption)
}

type tabSelectState struct {
	opts     TabSelectOptions
	selected int
	scroll   int // first visible tab
	// pinned keeps scroll after a click, so the clicked tab stays under
	// the pointer; other moves re-center as upstream.
	pinned bool
}

// TabSelect returns a focusable horizontal row of tabs, a port of upstream
// TabSelectRenderable: Left/[ and Right/] move, Enter selects. Each tab is
// TabWidth cells wide; names that do not fit end in "…". The selected tab
// is filled, underlined with ▬, and its description is shown on the row
// below. When tabs overflow, the view keeps the selection centered and
// draws ‹ and › arrows.
//
// The node is 1 row tall, plus 1 for the underline and 1 for the
// description, as upstream; any Style.Height is replaced by that.
//
// Beyond upstream: a click selects a tab (clicking the selected tab fires
// OnSelect), the wheel moves the selection, and an unsized node measures
// to the width of all its tabs.
func TabSelect(o TabSelectOptions) *Node {
	if o.FocusedBG == (RGBA{}) {
		if o.BG != (RGBA{}) {
			o.FocusedBG = o.BG
		} else {
			o.FocusedBG = MustHex("#1a1a1a")
		}
	}
	if o.FG == (RGBA{}) {
		o.FG = MustHex("#FFFFFF")
	}
	if o.FocusedFG == (RGBA{}) {
		o.FocusedFG = o.FG
	}
	defaults := []struct {
		c   *RGBA
		hex string
	}{
		{&o.SelectedBG, "#334455"}, {&o.SelectedFG, "#FFFF00"}, {&o.SelectedDescriptionColor, "#CCCCCC"},
	}
	for _, d := range defaults {
		if *d.c == (RGBA{}) {
			*d.c = MustHex(d.hex)
		}
	}
	if o.TabWidth <= 0 {
		o.TabWidth = 20
	}
	s := &tabSelectState{opts: o}
	o.Style.Height = Cells(s.height()) // upstream replaces any given height
	n := NewNode(o.Style)
	n.ID, n.ZIndex, n.Focusable = o.ID, o.ZIndex, true
	n.tabs = s
	s.selected = min(max(o.SelectedIndex, 0), max(len(o.Options)-1, 0))
	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		width := max(len(s.opts.Options), 1) * s.opts.TabWidth
		return fit(float64(width), w, wm), fit(float64(s.height()), h, hm)
	}
	n.paint = paintTabSelect
	n.keyHandler = n.handleTabSelectKey
	n.controlMouseHandler = n.handleTabSelectMouse
	return n
}

// height is upstream calculateDynamicHeight.
func (s *tabSelectState) height() int {
	h := 1
	if !s.opts.HideUnderline {
		h++
	}
	if !s.opts.HideDescription {
		h++
	}
	return h
}

func (s *tabSelectState) maxVisible(width int) int {
	return max(1, width/s.opts.TabWidth)
}

// center is upstream updateScrollOffset: keep the selection in the middle,
// unless a click selected a tab that is still in view.
func (s *tabSelectState) center(width int) {
	visible := s.maxVisible(width)
	if s.pinned && s.selected >= s.scroll && s.selected < s.scroll+visible {
		s.scroll = min(s.scroll, max(len(s.opts.Options)-visible, 0))
		return
	}
	s.pinned = false
	s.scroll = max(0, min(s.selected-visible/2, len(s.opts.Options)-visible))
}

// SetTabWidth changes how many cells each tab of a TabSelect takes.
func (n *Node) SetTabWidth(width int) {
	if n.tabs != nil && width > 0 {
		n.tabs.opts.TabWidth = width
		n.tabs.pinned = false // re-center, as upstream setTabWidth
	}
}

// TabWidth is the width in cells of each tab of a TabSelect.
func (n *Node) TabWidth() int {
	if n.tabs == nil {
		return 0
	}
	return n.tabs.opts.TabWidth
}

func (n *Node) moveTab(step int) {
	s := n.tabs
	count := len(s.opts.Options)
	if count == 0 {
		return
	}
	next := s.selected + step
	switch {
	case next >= 0 && next < count:
	case s.opts.WrapSelection && next < 0:
		next = count - 1
	case s.opts.WrapSelection:
		next = 0
	default:
		return
	}
	s.selected = next
	s.pinned = false
	n.notifyTabChange()
}

func (n *Node) notifyTabChange() {
	s := n.tabs
	if s.opts.OnChange != nil && s.selected < len(s.opts.Options) {
		s.opts.OnChange(s.selected, s.opts.Options[s.selected])
	}
}

func (n *Node) handleTabSelectKey(key *KeyEvent) {
	// Upstream bindings match modifiers exactly, and none has any.
	if n.tabs == nil || key.Type == KeyRelease || key.Ctrl || key.Alt || key.Meta || key.Super || key.Shift {
		return
	}
	switch key.Name {
	case "left", "[":
		n.moveTab(-1)
	case "right", "]":
		n.moveTab(1)
	case "return", "linefeed":
		n.SelectCurrent()
	default:
		return
	}
	key.PreventDefault()
}

// tabAt is the option index of the tab under (x, y), or -1. Only the tab
// row and its underline hold tabs; the description row does not.
func (n *Node) tabAt(x, y int) int {
	s := n.tabs
	r := n.rect.Inset(n.Style.Padding)
	rows := 1
	if !s.opts.HideUnderline {
		rows = 2
	}
	if x < r.X || x >= r.X+r.W || y < r.Y || y >= r.Y+min(rows, r.H) {
		return -1
	}
	slot := (x - r.X) / s.opts.TabWidth
	if slot >= s.maxVisible(r.W) {
		return -1
	}
	i := s.scroll + slot
	if i >= len(s.opts.Options) {
		return -1
	}
	return i
}

func (n *Node) handleTabSelectMouse(e *MouseEvent) {
	s := n.tabs
	switch e.Type {
	case MouseDown:
		if e.Button != MouseLeft {
			return
		}
		if i := n.tabAt(e.X, e.Y); i >= 0 {
			if i == s.selected {
				n.SelectCurrent()
			} else {
				n.SetSelectedIndex(i)
				s.pinned = true
			}
			e.StopPropagation()
		}
	case MouseScroll:
		if e.Scroll == nil {
			return
		}
		before := s.selected
		switch e.Scroll.Direction {
		case ScrollUp, ScrollLeft:
			n.moveTab(-1)
		case ScrollDown, ScrollRight:
			n.moveTab(1)
		}
		if s.selected != before {
			e.PreventDefault()
			e.StopPropagation()
		}
	}
}

var tabArrowColor = MustHex("#AAAAAA")

// drawTextFit draws text in at most maxWidth cells from x, as upstream
// drawTextFit with ellipsis: text wider than maxWidth keeps the whole
// grapheme clusters that leave room for "…", then draws it.
func drawTextFit(b *Buffer, text string, x, y, maxWidth int, fg RGBA) {
	if maxWidth < 1 {
		return
	}
	if displayWidth(text) > maxWidth {
		var kept strings.Builder
		for _, c := range clusters(text) {
			if c.end > maxWidth-1 {
				break
			}
			kept.WriteString(c.text)
		}
		text = kept.String() + "…"
	}
	b.PushClip(Rect{X: x, Y: y, W: maxWidth, H: 1})
	b.DrawText(text, x, y, fg, Transparent, AttrNone)
	b.PopClip()
}

// paintTabSelect draws as upstream TabSelectRenderable.renderSelf.
func paintTabSelect(b *Buffer, n *Node) {
	s := n.tabs
	r := n.rect.Inset(n.Style.Padding)
	if r.Empty() {
		return
	}
	o := &s.opts
	b.PushClip(r)
	defer b.PopClip()
	bg, fg := o.BG, o.FG
	if n.focused {
		bg, fg = o.FocusedBG, o.FocusedFG
	}
	if bg.A > 0 {
		b.FillRect(r.X, r.Y, r.W, r.H, bg)
	}
	s.selected = min(s.selected, max(len(o.Options)-1, 0))
	s.center(r.W)
	visible := s.maxVisible(r.W)
	selectedRows := 1
	if !o.HideUnderline && r.H >= 2 {
		selectedRows = 2
	}

	for i := 0; i < visible && s.scroll+i < len(o.Options); i++ {
		tabX := r.X + i*o.TabWidth
		if tabX >= r.X+r.W {
			break
		}
		width := min(o.TabWidth, r.W-i*o.TabWidth)
		selected := s.scroll+i == s.selected
		color := fg
		if selected {
			color = o.SelectedFG
			b.FillRect(tabX, r.Y, width, selectedRows, o.SelectedBG)
		}
		drawTextFit(b, o.Options[s.scroll+i].Name, tabX+1, r.Y, width-2, color)
		if selected && selectedRows == 2 {
			b.DrawText(strings.Repeat("▬", width), tabX, r.Y+1, color, Transparent, AttrNone)
		}
	}

	descRow := 1
	if !o.HideUnderline {
		descRow = 2
	}
	if !o.HideDescription && r.H > descRow && s.selected < len(o.Options) {
		drawTextFit(b, o.Options[s.selected].Description, r.X+1, r.Y+descRow, r.W-2, o.SelectedDescriptionColor)
	}

	if !o.HideScrollArrows && len(o.Options) > visible {
		if s.scroll > 0 {
			b.DrawText("‹", r.X, r.Y, tabArrowColor, Transparent, AttrNone)
		}
		if s.scroll+visible < len(o.Options) {
			b.DrawText("›", r.X+r.W-1, r.Y, tabArrowColor, Transparent, AttrNone)
		}
	}
}
