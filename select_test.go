package opentui

import (
	"fmt"
	"strings"
	"testing"
)

func fruitOptions() []SelectOption {
	return []SelectOption{
		{Name: "alpha", Description: "first", Value: 1},
		{Name: "beta", Description: "second", Value: 2},
		{Name: "bat", Description: "third", Value: 3},
	}
}

func selectAt(o SelectOptions, w, h int) (*Renderer, *SelectNode, *Buffer) {
	o.Style.Width, o.Style.Height = Cells(w), Cells(h)
	n := Select(o)
	r := testRenderer(NewNode(Style{AlignItems: AlignStart}, n))
	b := NewBuffer(w, h)
	r.drawFrame(b)
	return r, n, b
}

func rowText(b *Buffer, y int) string {
	var s strings.Builder
	for x := 0; x < b.Width(); x++ {
		c, _ := b.Get(x, y)
		switch {
		case c.Continuation:
		case c.Grapheme() != "":
			s.WriteString(c.Grapheme())
		default:
			s.WriteRune(c.Char)
		}
	}
	return strings.TrimRight(s.String(), " ")
}

func TestSelectPaintsLikeUpstream(t *testing.T) {
	_, n, b := selectAt(SelectOptions{Options: fruitOptions()}, 12, 6)
	want := []string{" ▶ alpha", "   first", "   beta", "   second", "   bat", "   third"}
	for y, line := range want {
		if got := rowText(b, y); got != line {
			t.Fatalf("row %d = %q, want %q", y, got, line)
		}
	}
	o := n.selectState().opts
	if c, _ := b.Get(5, 0); c.BG != o.SelectedBG || c.FG != o.SelectedFG {
		t.Fatalf("selected name cell: %+v", c)
	}
	if c, _ := b.Get(4, 1); c.BG != o.SelectedBG || c.FG != o.SelectedDescriptionColor {
		t.Fatalf("selected description cell: %+v", c)
	}
	if c, _ := b.Get(4, 2); c.BG == o.SelectedBG || c.FG != o.FG {
		t.Fatalf("unselected cell: %+v", c)
	}
}

func TestSelectKeysAndEvents(t *testing.T) {
	var changes, selects []string
	opts := SelectOptions{
		Options:  fruitOptions(),
		OnChange: func(i int, o SelectOption) { changes = append(changes, fmt.Sprintf("%d %s", i, o.Name)) },
		OnSelect: func(i int, o SelectOption) { selects = append(selects, fmt.Sprint(i, o.Value)) },
	}
	n := Select(opts)
	n.keyHandler(key("down", "\x1b[B"))
	n.keyHandler(key("j", "j"))
	n.keyHandler(key("j", "j")) // at the end without wrapping: stays, but still reports a change
	n.keyHandler(key("k", "k"))
	k := key("return", "\r")
	n.keyHandler(k)
	if !k.DefaultPrevented() {
		t.Fatal("Enter not handled")
	}
	if got := strings.Join(changes, ","); got != "1 beta,2 bat,2 bat,1 beta" {
		t.Fatalf("changes: %s", got)
	}
	if got := strings.Join(selects, ","); got != "1 2" {
		t.Fatalf("selects: %s", got)
	}
	if n.SelectedIndex() != 1 {
		t.Fatalf("index %d", n.SelectedIndex())
	}
	other := key("x", "x")
	n.keyHandler(other)
	if other.DefaultPrevented() {
		t.Fatal("unbound key was handled")
	}

	wrap := Select(SelectOptions{Options: fruitOptions(), WrapSelection: true})
	wrap.keyHandler(key("up", "\x1b[A"))
	if wrap.SelectedIndex() != 2 {
		t.Fatalf("wrap up: %d", wrap.SelectedIndex())
	}
	wrap.keyHandler(key("down", "\x1b[B"))
	if wrap.SelectedIndex() != 0 {
		t.Fatalf("wrap down: %d", wrap.SelectedIndex())
	}
}

func numbered(count int) []SelectOption {
	var out []SelectOption
	for i := 0; i < count; i++ {
		out = append(out, SelectOption{Name: fmt.Sprintf("item%d", i)})
	}
	return out
}

func TestSelectScrollCentersAndFastSteps(t *testing.T) {
	r, n, _ := selectAt(SelectOptions{Options: numbered(10), HideDescription: true, ShowScrollIndicator: true}, 10, 3)
	n.keyHandler(key("down", "\x1b[1;2B", "shift")) // fast step of 5
	if n.SelectedIndex() != 5 {
		t.Fatalf("fast down: %d", n.SelectedIndex())
	}
	b := NewBuffer(10, 3)
	r.drawFrame(b)
	for y, want := range []string{"   item4", " ▶ item5", "   item6"} {
		if got := strings.TrimRight(rowText(b, y), "█ "); got != want {
			t.Fatalf("row %d = %q, want %q (centered on the selection)", y, got, want)
		}
	}
	// Scroll indicator: offset 4 of 7, on rows y+1 .. y+h-2.
	if c, _ := b.Get(9, 1); c.Char != '█' {
		t.Fatalf("scroll indicator missing: %+v", c)
	}
	n.keyHandler(key("down", "\x1b[1;2B", "shift"))
	b = NewBuffer(10, 3)
	r.drawFrame(b)
	if got := strings.TrimRight(rowText(b, 2), "█ "); got != " ▶ item9" {
		t.Fatalf("clamped at the end: %q", got)
	}
}

func TestSelectFilter(t *testing.T) {
	var changes []int
	n := Select(SelectOptions{Options: fruitOptions(), OnChange: func(i int, _ SelectOption) { changes = append(changes, i) }})
	n.SetFilter("B") // case-insensitive, matches beta and bat
	if n.SelectedIndex() != 1 {
		t.Fatalf("selection did not move to the first match: %d", n.SelectedIndex())
	}
	n.keyHandler(key("down", "\x1b[B"))
	if n.SelectedIndex() != 2 {
		t.Fatalf("down within the filter: %d", n.SelectedIndex())
	}
	n.keyHandler(key("down", "\x1b[B"))
	if n.SelectedIndex() != 2 {
		t.Fatalf("down at the end of the filter: %d", n.SelectedIndex())
	}
	n.SetFilter("third") // descriptions match too
	if n.SelectedIndex() != 2 || n.VisibleOptions() != 1 {
		t.Fatalf("description filter: index %d, visible %d", n.SelectedIndex(), n.VisibleOptions())
	}
	n.SetFilter("zzz")
	if _, ok := n.SelectedOption(); ok || n.VisibleOptions() != 0 {
		t.Fatal("no match must mean no selected option")
	}
	n.SetFilter("")
	if n.SelectedIndex() != 2 || n.VisibleOptions() != 3 {
		t.Fatalf("clearing the filter: index %d, visible %d", n.SelectedIndex(), n.VisibleOptions())
	}
	// The snap to the first match and the two Down keys; filters that keep
	// the selection report nothing.
	if fmt.Sprint(changes) != "[1 2 2]" {
		t.Fatalf("changes %v", changes)
	}
}

func TestSelectMouse(t *testing.T) {
	var selected []int
	r, n, _ := selectAt(SelectOptions{Options: fruitOptions(), OnSelect: func(i int, _ SelectOption) { selected = append(selected, i) }}, 12, 6)
	click(r, 2, 4) // "bat" name row
	if n.SelectedIndex() != 2 || len(selected) != 0 || !n.Focused() {
		t.Fatalf("click: index %d, selects %v, focused %v", n.SelectedIndex(), selected, n.Focused())
	}
	click(r, 2, 5) // its description row: the same item, already selected
	if len(selected) != 1 || selected[0] != 2 {
		t.Fatalf("click on the selected item: %v", selected)
	}
	if e := wheel(r, 2, 2, ScrollUp); !e.PropagationStopped() || n.SelectedIndex() != 1 {
		t.Fatalf("wheel up: index %d", n.SelectedIndex())
	}
	wheel(r, 2, 2, ScrollUp)
	if e := wheel(r, 2, 2, ScrollUp); e.PropagationStopped() {
		t.Fatal("wheel at the first item must bubble")
	}
}

func TestSelectEdgeCases(t *testing.T) {
	empty := Select(SelectOptions{})
	empty.keyHandler(key("down", "\x1b[B"))
	if _, ok := empty.SelectedOption(); ok {
		t.Fatal("empty select has an option")
	}
	draw(NewNode(Style{}, empty), 5, 2) // must not panic

	n := Select(SelectOptions{Options: fruitOptions(), SelectedIndex: 9})
	if n.SelectedIndex() != 2 {
		t.Fatalf("initial index not clamped: %d", n.SelectedIndex())
	}
	n.SetSelectOptions(fruitOptions()[:1])
	if n.SelectedIndex() != 0 {
		t.Fatalf("index not clamped after SetSelectOptions: %d", n.SelectedIndex())
	}
	ComputeLayout(NewNode(Style{AlignItems: AlignStart}, n), 40, 20)
	if r := n.Rect(); r.W != 9 || r.H != 2 { // " ▶ alpha " and its description
		t.Fatalf("natural size %dx%d, want 9x2", r.W, r.H)
	}

	_, _, b := selectAt(SelectOptions{Options: []SelectOption{{Name: "ab界"}}, HideDescription: true, HideSelectionIndicator: true}, 4, 1)
	if got := rowText(b, 0); got != " ab" {
		t.Fatalf("wide glyph at the edge: %q", got)
	}
}

func FuzzSelect(f *testing.F) {
	f.Add(uint8(10), "item", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, uint8(10), uint8(4), true)
	f.Add(uint8(0), "", []byte{0, 1, 2}, uint8(1), uint8(1), false)
	keys := []*KeyEvent{
		key("up", "\x1b[A"), key("down", "\x1b[B"), key("k", "k"), key("j", "j"),
		key("up", "\x1b[1;2A", "shift"), key("down", "\x1b[1;2B", "shift"), key("return", "\r"),
	}
	f.Fuzz(func(t *testing.T, count uint8, filter string, ops []byte, w, h uint8, desc bool) {
		var options []SelectOption
		for i := 0; i < int(count%40); i++ {
			options = append(options, SelectOption{Name: fmt.Sprintf("item%d 界", i), Description: fmt.Sprintf("d%d", i%7)})
		}
		width, height := int(w%20)+1, int(h%8)+1
		r, n, _ := selectAt(SelectOptions{Options: options, HideDescription: !desc, ShowScrollIndicator: true,
			WrapSelection: count%2 == 0, ItemSpacing: int(count % 3)}, width, height)
		for i, op := range ops[:min(len(ops), 200)] {
			switch op % 5 {
			case 0:
				r.dispatchMouse(&MouseEvent{Type: MouseDown, Button: MouseLeft, X: int(op/5) % width, Y: int(op/5) % height})
				r.dispatchMouse(&MouseEvent{Type: MouseUp, Button: MouseLeft, X: int(op/5) % width, Y: int(op/5) % height})
			case 1:
				dir := ScrollUp
				if op&8 != 0 {
					dir = ScrollDown
				}
				wheel(r, 0, 0, dir)
			case 2:
				n.SetFilter(filter[:min(len(filter), i%4)])
			default:
				k := *keys[int(op/5)%len(keys)]
				n.keyHandler(&k)
			}
			r.drawFrame(NewBuffer(width, height))
			s := n.list
			if len(options) > 0 && (s.selected < 0 || s.selected >= len(options)) {
				t.Fatalf("selected %d of %d", s.selected, len(options))
			}
			if len(s.visible) > 0 && s.position() < 0 {
				t.Fatalf("selection %d hidden by filter %q", s.selected, s.filter)
			}
			if s.scroll < 0 || (len(s.visible) > 0 && s.scroll >= len(s.visible)) {
				t.Fatalf("scroll %d of %d visible", s.scroll, len(s.visible))
			}
		}
	})
}

// A click must not scroll the item away from the pointer: upstream
// re-centers on every selection change, which suits the keyboard but made
// a second click on the same spot land on a different item.
func TestSelectClickKeepsItemUnderPointer(t *testing.T) {
	var chosen []string
	r, n, _ := selectAt(SelectOptions{Options: numbered(12), HideDescription: true,
		OnSelect: func(_ int, o SelectOption) { chosen = append(chosen, o.Name) }}, 10, 5)
	// Centered on item0: rows show item0..item4. Click item4 at row 4.
	click(r, 3, 4)
	if n.SelectedIndex() != 4 {
		t.Fatalf("click selected %d", n.SelectedIndex())
	}
	b := NewBuffer(10, 5)
	r.drawFrame(b)
	if got := rowText(b, 4); got != " ▶ item4" {
		t.Fatalf("item moved away from the pointer: row 4 = %q", got)
	}
	click(r, 3, 4)
	if len(chosen) != 1 || chosen[0] != "item4" {
		t.Fatalf("second click chose %v, want [item4]", chosen)
	}
	n.keyHandler(key("down", "\x1b[B")) // the keyboard re-centers again
	b = NewBuffer(10, 5)
	r.drawFrame(b)
	if got := rowText(b, 2); got != " ▶ item5" {
		t.Fatalf("keyboard move did not re-center: row 2 = %q", got)
	}
}
