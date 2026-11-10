package opentui

import "strings"

// SelectOption is one item of a Select, as upstream SelectOption.
type SelectOption struct {
	Name        string
	Description string
	Value       any
}

// SelectOptions configures a Select. Zero colors use upstream's defaults.
type SelectOptions struct {
	ID     string
	Style  Style
	ZIndex int

	Options       []SelectOption
	SelectedIndex int

	BG, FG                   RGBA // unfocused; BG zero is transparent
	FocusedBG, FocusedFG     RGBA
	SelectedBG, SelectedFG   RGBA
	DescriptionColor         RGBA
	SelectedDescriptionColor RGBA

	ShowScrollIndicator bool // a █ at the right edge when items scroll
	WrapSelection       bool // moving past either end wraps around
	// HideDescription and HideSelectionIndicator turn off what upstream's
	// showDescription and showSelectionIndicator (default true) show.
	HideDescription        bool
	HideSelectionIndicator bool
	ItemSpacing            int // blank rows after each item
	FastScrollStep         int // Shift+Up/Down step; zero uses 5

	// OnChange is called after every move, with the index into Options.
	// Like upstream selectionChanged it also fires when a move stays put
	// at an end.
	OnChange func(index int, option SelectOption)
	// OnSelect is called on Enter (upstream itemSelected) and when the
	// already-selected item is clicked.
	OnSelect func(index int, option SelectOption)
}

type selectState struct {
	opts     SelectOptions
	selected int   // index into opts.Options
	visible  []int // indexes matching the filter, in order
	filter   string
	scroll   int // first visible item, as a position in visible
	// pinned keeps scroll after a mouse selection, so the clicked item
	// stays under the pointer; keyboard moves re-center as upstream.
	pinned bool
}

// Select returns a focusable list of options, a port of upstream
// SelectRenderable: Up/K and Down/J move, Shift+Up/Down move by
// FastScrollStep, Enter selects. The view keeps the selection centered.
//
// Beyond upstream: a click selects an item (clicking the selected item
// fires OnSelect), the wheel moves the selection, SetFilter narrows the
// list, and the node measures to its content when no size is given.
func Select(o SelectOptions) *SelectNode {
	defaults := []struct {
		c   *RGBA
		hex string
	}{
		{&o.FG, "#FFFFFF"}, {&o.FocusedBG, "#1a1a1a"}, {&o.FocusedFG, "#FFFFFF"},
		{&o.SelectedBG, "#334455"}, {&o.SelectedFG, "#FFFF00"},
		{&o.DescriptionColor, "#888888"}, {&o.SelectedDescriptionColor, "#CCCCCC"},
	}
	for _, d := range defaults {
		if *d.c == (RGBA{}) {
			*d.c = MustHex(d.hex)
		}
	}
	if o.FastScrollStep <= 0 {
		o.FastScrollStep = 5
	}
	n := NewNode(o.Style)
	n.ID, n.ZIndex, n.Focusable = o.ID, o.ZIndex, true
	s := &selectState{opts: o}
	n.list = s
	n.setSelectOptions(o.Options)
	s.selected = min(max(o.SelectedIndex, 0), max(len(o.Options)-1, 0))
	n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
		width := 0
		for _, i := range s.visible {
			opt := s.opts.Options[i]
			width = max(width, displayWidth(opt.Name)+s.indicatorWidth(), displayWidth(opt.Description)+s.indicatorWidth())
		}
		height := len(s.visible) * s.linesPerItem()
		return fit(float64(width+2), w, wm), fit(float64(max(height, 1)), h, hm)
	}
	n.paint = paintSelect
	n.keyHandler = n.handleSelectKey
	n.controlMouseHandler = n.handleSelectMouse
	return &SelectNode{n}
}

func (n *Node) selectState() *selectState { return n.list }

func (s *selectState) indicatorWidth() int {
	if s.opts.HideSelectionIndicator {
		return 0
	}
	return 2
}

func (s *selectState) linesPerItem() int {
	lines := 1 + s.opts.ItemSpacing
	if !s.opts.HideDescription {
		lines++
	}
	return lines
}

// position is the selected item's place in the visible list, or -1.
func (s *selectState) position() int {
	for p, i := range s.visible {
		if i == s.selected {
			return p
		}
	}
	return -1
}

// SetSelectOptions replaces a Select's or TabSelect's options, keeping the
// selected index when it still exists, as upstream's options setter.
func (n *Node) setSelectOptions(options []SelectOption) {
	if t := n.tabs; t != nil {
		t.opts.Options = options
		t.selected = min(t.selected, max(len(options)-1, 0))
		t.pinned = false
		return
	}
	s := n.list
	if s == nil {
		return
	}
	s.opts.Options = options
	s.selected = min(s.selected, max(len(options)-1, 0))
	n.applyFilter(false)
}

// SetFilter shows only options whose name or description contains query,
// ignoring case ("" shows all). Indexes still refer to the full option
// list. If the selection is filtered out, it moves to the first match and
// OnChange fires.
func (n *Node) setFilter(query string) {
	if n.list != nil {
		n.list.filter = query
		n.list.pinned = false
		n.applyFilter(true)
	}
}

func (n *Node) applyFilter(notify bool) {
	s := n.list
	q := strings.ToLower(s.filter)
	s.visible = s.visible[:0]
	for i, o := range s.opts.Options {
		if q == "" || strings.Contains(strings.ToLower(o.Name), q) || strings.Contains(strings.ToLower(o.Description), q) {
			s.visible = append(s.visible, i)
		}
	}
	if len(s.visible) > 0 && s.position() < 0 {
		s.selected = s.visible[0]
		if notify {
			n.notifySelectChange()
		}
	}
}

// VisibleOptions is how many options pass the filter.
func (n *Node) visibleOptions() int {
	if n.list == nil {
		return 0
	}
	return len(n.list.visible)
}

// SelectedIndex is the index into the options of a Select's or
// TabSelect's selection.
func (n *Node) selectedIndex() int {
	if n.tabs != nil {
		return n.tabs.selected
	}
	if n.list == nil {
		return 0
	}
	return n.list.selected
}

// SelectedOption returns the selected option; false when the list is
// empty or the filter hides everything.
func (n *Node) selectedOption() (SelectOption, bool) {
	if t := n.tabs; t != nil {
		if t.selected >= len(t.opts.Options) {
			return SelectOption{}, false
		}
		return t.opts.Options[t.selected], true
	}
	s := n.list
	if s == nil || s.position() < 0 {
		return SelectOption{}, false
	}
	return s.opts.Options[s.selected], true
}

// SetSelectedIndex selects option index if it exists and is visible, and
// fires OnChange, even when index is already selected, as upstream.
func (n *Node) setSelectedIndex(index int) {
	if t := n.tabs; t != nil {
		if index >= 0 && index < len(t.opts.Options) {
			t.selected, t.pinned = index, false
			n.notifyTabChange()
		}
		return
	}
	s := n.list
	if s == nil {
		return
	}
	for _, i := range s.visible {
		if i == index {
			s.selected = index
			n.notifySelectChange()
			return
		}
	}
}

// MoveSelection moves the selection by steps visible items (negative is
// up or left), clamping or wrapping at the ends as upstream
// moveUp/moveDown, and fires OnChange. A TabSelect moves one tab per step
// and, as upstream moveLeft/moveRight, fires OnChange only for steps that
// move.
func (n *Node) moveSelection(steps int) {
	if n.tabs != nil {
		step := 1
		if steps < 0 {
			step, steps = -1, -steps
		}
		for ; steps > 0; steps-- {
			n.moveTab(step)
		}
		return
	}
	s := n.list
	if s == nil || len(s.visible) == 0 {
		return
	}
	p := max(s.position(), 0) + steps
	last := len(s.visible) - 1
	switch {
	case p < 0 && s.opts.WrapSelection:
		p = last
	case p > last && s.opts.WrapSelection:
		p = 0
	}
	s.selected = s.visible[min(max(p, 0), last)]
	s.pinned = false
	n.notifySelectChange()
}

// SelectCurrent fires OnSelect for the selected option, if any.
func (n *Node) selectCurrent() bool {
	opt, ok := n.selectedOption()
	var onSelect func(int, SelectOption)
	switch {
	case !ok:
	case n.tabs != nil:
		onSelect = n.tabs.opts.OnSelect
	case n.list != nil:
		onSelect = n.list.opts.OnSelect
	}
	if onSelect != nil {
		onSelect(n.selectedIndex(), opt)
	}
	return ok
}

func (n *Node) notifySelectChange() {
	s := n.list
	if s.opts.OnChange != nil && s.position() >= 0 {
		s.opts.OnChange(s.selected, s.opts.Options[s.selected])
	}
}

func (n *Node) handleSelectKey(key *KeyEvent) {
	if n.list == nil || key.Type == KeyRelease || key.Ctrl || key.Alt || key.Meta || key.Super {
		return
	}
	switch {
	case key.Name == "up" && key.Shift:
		n.moveSelection(-n.list.opts.FastScrollStep)
	case key.Name == "down" && key.Shift:
		n.moveSelection(n.list.opts.FastScrollStep)
	case key.Name == "up" || key.Name == "k":
		n.moveSelection(-1)
	case key.Name == "down" || key.Name == "j":
		n.moveSelection(1)
	case key.Name == "return" || key.Name == "linefeed":
		n.selectCurrent()
	default:
		return
	}
	key.PreventDefault()
}

// itemAt is the option index under screen row y, or -1.
func (n *Node) itemAt(y int) int {
	s := n.list
	r := n.rect.Inset(n.Style.Padding)
	if y < r.Y || y >= r.Y+r.H {
		return -1
	}
	p := s.scroll + (y-r.Y)/s.linesPerItem()
	if p >= len(s.visible) {
		return -1
	}
	return s.visible[p]
}

func (n *Node) handleSelectMouse(e *MouseEvent) {
	s := n.list
	switch e.Type {
	case MouseDown:
		if e.Button != MouseLeft {
			return
		}
		if i := n.itemAt(e.Y); i >= 0 {
			if i == s.selected {
				n.selectCurrent()
			} else {
				s.pinned = true
				n.setSelectedIndex(i)
			}
			e.StopPropagation()
		}
	case MouseScroll:
		if e.Scroll == nil || len(s.visible) == 0 {
			return
		}
		before := s.selected
		switch e.Scroll.Direction {
		case ScrollUp:
			if s.position() > 0 || s.opts.WrapSelection {
				n.moveSelection(-1)
			}
		case ScrollDown:
			if s.position() < len(s.visible)-1 || s.opts.WrapSelection {
				n.moveSelection(1)
			}
		}
		if s.selected != before {
			e.PreventDefault()
			e.StopPropagation()
		}
	}
}

var selectIndicatorColor = MustHex("#666666")

// paintSelect draws as upstream SelectRenderable.renderSelf: background,
// the selected item's fill, "▶ " marker, name, dimmer description on the
// next row, and an optional scroll indicator.
func paintSelect(b *Buffer, n *Node) {
	s := n.list
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

	lines := s.linesPerItem()
	maxVisible := max(1, r.H/lines)
	// Keep the selection centered, as upstream updateScrollOffset, unless
	// a click selected an item that is already in view.
	if p := s.position(); p < 0 {
		s.scroll = 0
	} else if !s.pinned || p < s.scroll || p >= s.scroll+maxVisible {
		s.scroll = max(0, min(p-maxVisible/2, len(s.visible)-maxVisible))
		s.pinned = false
	}
	s.scroll = min(s.scroll, max(len(s.visible)-maxVisible, 0))
	scrolls := o.ShowScrollIndicator && len(s.visible) > maxVisible
	indicatorY := -1
	if scrolls {
		percent := float64(s.scroll) / float64(len(s.visible)-maxVisible)
		indicatorY = r.Y + 1 + int(percent*float64(max(1, r.H-2)))
	}
	// Text on the indicator's row stops before it, so the indicator never
	// covers half of a wide glyph.
	text := func(t string, x, y int, color RGBA) {
		end := r.X + r.W
		if y == indicatorY {
			end--
		}
		if end > x {
			b.PushClip(Rect{X: x, Y: y, W: end - x, H: 1})
			b.DrawText(t, x, y, color, Transparent, AttrNone)
			b.PopClip()
		}
	}

	textX := r.X + 1 + s.indicatorWidth()
	for i := 0; i < maxVisible && s.scroll+i < len(s.visible); i++ {
		index := s.visible[s.scroll+i]
		opt := o.Options[index]
		y := r.Y + i*lines
		if y+lines-1 >= r.Y+r.H {
			break
		}
		selected := index == s.selected
		nameColor, descColor := fg, o.DescriptionColor
		if selected {
			b.FillRect(r.X, y, r.W, lines-o.ItemSpacing, o.SelectedBG)
			nameColor, descColor = o.SelectedFG, o.SelectedDescriptionColor
		}
		indicator := ""
		if !o.HideSelectionIndicator {
			indicator = "  "
			if selected {
				indicator = "▶ "
			}
		}
		text(indicator+opt.Name, r.X+1, y, nameColor)
		if !o.HideDescription && y+1 < r.Y+r.H {
			text(opt.Description, textX, y+1, descColor)
		}
	}
	if scrolls {
		b.DrawText("█", r.X+r.W-1, indicatorY, selectIndicatorColor, Transparent, AttrNone)
	}
}
