package opentui

import (
	"fmt"
	"strings"
	"testing"
)

func tabsAt(o TabSelectOptions, w int) (*Renderer, *Node, *Buffer) {
	o.Style.Width = Cells(w)
	n := TabSelect(o)
	r := testRenderer(NewNode(Style{AlignItems: AlignStart}, n))
	b := NewBuffer(w, 3)
	r.drawFrame(b)
	return r, n, b
}

func redraw(r *Renderer, w, h int) *Buffer {
	b := NewBuffer(w, h)
	r.drawFrame(b)
	return b
}

func TestTabSelectPaintsLikeUpstream(t *testing.T) {
	_, n, b := tabsAt(TabSelectOptions{Options: fruitOptions(), TabWidth: 8}, 24)
	want := []string{" alpha   beta    bat", "▬▬▬▬▬▬▬▬", " first"}
	for y, line := range want {
		if got := rowText(b, y); got != line {
			t.Fatalf("row %d = %q, want %q", y, got, line)
		}
	}
	if r := n.Rect(); r.H != 3 {
		t.Fatalf("height %d, want 3 (tabs, underline, description)", r.H)
	}
	o := n.tabs.opts
	if c, _ := b.Get(1, 0); c.BG != o.SelectedBG || c.FG != o.SelectedFG {
		t.Fatalf("selected name cell: %+v", c)
	}
	if c, _ := b.Get(7, 1); c.BG != o.SelectedBG || c.FG != o.SelectedFG {
		t.Fatalf("underline cell: %+v", c)
	}
	if c, _ := b.Get(9, 0); c.BG == o.SelectedBG || c.FG != o.FG {
		t.Fatalf("unselected name cell: %+v", c)
	}
	if c, _ := b.Get(1, 2); c.FG != o.SelectedDescriptionColor {
		t.Fatalf("description cell: %+v", c)
	}
}

func TestTabSelectHeightOptions(t *testing.T) {
	for _, tc := range []struct {
		o    TabSelectOptions
		want int
	}{
		{TabSelectOptions{}, 3},
		{TabSelectOptions{HideUnderline: true}, 2},
		{TabSelectOptions{HideDescription: true}, 2},
		{TabSelectOptions{HideUnderline: true, HideDescription: true}, 1},
		{TabSelectOptions{Style: Style{Height: Cells(10)}}, 3}, // replaced, as upstream
	} {
		tc.o.Options = fruitOptions()
		n := TabSelect(tc.o)
		ComputeLayout(NewNode(Style{AlignItems: AlignStart}, n), 80, 10)
		if r := n.Rect(); r.H != tc.want || r.W != 60 {
			t.Fatalf("%+v: %dx%d, want 60x%d", tc.o, r.W, r.H, tc.want)
		}
	}
	// Without the underline, the description moves up a row.
	_, _, b := tabsAt(TabSelectOptions{Options: fruitOptions(), TabWidth: 8, HideUnderline: true}, 24)
	if got := rowText(b, 1); got != " first" {
		t.Fatalf("description row: %q", got)
	}
}

func TestTabSelectKeysAndEvents(t *testing.T) {
	var changes, selects []string
	n := TabSelect(TabSelectOptions{
		Options:  fruitOptions(),
		OnChange: func(i int, o SelectOption) { changes = append(changes, fmt.Sprintf("%d %s", i, o.Name)) },
		OnSelect: func(i int, o SelectOption) { selects = append(selects, fmt.Sprint(i, o.Value)) },
	})
	n.keyHandler(key("right", "\x1b[C"))
	n.keyHandler(key("]", "]"))
	end := key("right", "\x1b[C") // at the end without wrapping: no change, as upstream
	n.keyHandler(end)
	if !end.DefaultPrevented() {
		t.Fatal("Right at the end must still be consumed")
	}
	n.keyHandler(key("[", "["))
	k := key("return", "\r")
	n.keyHandler(k)
	if !k.DefaultPrevented() {
		t.Fatal("Enter not handled")
	}
	if got := strings.Join(changes, ","); got != "1 beta,2 bat,1 beta" {
		t.Fatalf("changes: %s", got)
	}
	if got := strings.Join(selects, ","); got != "1 2" {
		t.Fatalf("selects: %s", got)
	}
	for _, k := range []*KeyEvent{key("up", "\x1b[A"), key("x", "x"), key("left", "\x1b[1;2D", "shift"), key("left", "\x1b[1;5D", "ctrl"),
		key("return", "\x1b[13;2u", "shift"), key("]", "\x1b[93;2u", "shift")} {
		n.keyHandler(k)
		if k.DefaultPrevented() || n.SelectedIndex() != 1 {
			t.Fatalf("unbound key %q was handled", k.Name)
		}
	}

	wrap := TabSelect(TabSelectOptions{Options: fruitOptions(), WrapSelection: true})
	wrap.keyHandler(key("left", "\x1b[D"))
	if wrap.SelectedIndex() != 2 {
		t.Fatalf("wrap left: %d", wrap.SelectedIndex())
	}
	wrap.keyHandler(key("right", "\x1b[C"))
	if wrap.SelectedIndex() != 0 {
		t.Fatalf("wrap right: %d", wrap.SelectedIndex())
	}
}

func TestTabSelectSharedSelectionAPI(t *testing.T) {
	var changes []int
	n := TabSelect(TabSelectOptions{Options: numbered(6), OnChange: func(i int, _ SelectOption) { changes = append(changes, i) }})
	n.MoveSelection(3)
	n.MoveSelection(9) // clamps at the end; only the moves that happen report
	n.SetSelectedIndex(1)
	n.SetSelectedIndex(1) // reports again, as upstream setSelectedIndex
	n.SetSelectedIndex(99)
	if fmt.Sprint(changes) != "[1 2 3 4 5 1 1]" {
		t.Fatalf("changes %v", changes)
	}
	if o, ok := n.SelectedOption(); !ok || o.Name != "item1" {
		t.Fatalf("selected option %+v %v", o, ok)
	}
	n.SetSelectOptions(numbered(1))
	if n.SelectedIndex() != 0 {
		t.Fatalf("index not clamped after SetSelectOptions: %d", n.SelectedIndex())
	}
	if n.TabWidth() != 20 {
		t.Fatalf("default tab width %d", n.TabWidth())
	}
}

func TestTabSelectScrollsAndCenters(t *testing.T) {
	r, n, b := tabsAt(TabSelectOptions{Options: numbered(10), TabWidth: 7, HideDescription: true}, 21)
	if got := rowText(b, 0); got != " item0  item1  item2›" {
		t.Fatalf("first page: %q", got)
	}
	n.SetSelectedIndex(5)
	b = redraw(r, 21, 3)
	// Three tabs fit; the selection is the middle one.
	if got := rowText(b, 0); got != "‹item4  item5  item6›" {
		t.Fatalf("centered: %q", got)
	}
	if got := rowText(b, 1); got != "       ▬▬▬▬▬▬▬" {
		t.Fatalf("underline under the middle tab: %q", got)
	}
	n.SetSelectedIndex(9)
	b = redraw(r, 21, 3)
	if got := rowText(b, 0); got != "‹item7  item8  item9" {
		t.Fatalf("clamped at the end: %q", got)
	}
	_, _, b = tabsAt(TabSelectOptions{Options: numbered(10), TabWidth: 7, HideScrollArrows: true}, 21)
	if got := rowText(b, 0); got != " item0  item1  item2" {
		t.Fatalf("without arrows: %q", got)
	}
}

func TestTabSelectTruncatesWithEllipsis(t *testing.T) {
	opts := []SelectOption{{Name: "overview", Description: "a long description here"}, {Name: "界界界界"}, {Name: "ab"}}
	_, _, b := tabsAt(TabSelectOptions{Options: opts, TabWidth: 7}, 21)
	// Tab text has TabWidth-2 = 5 cells: "overview" → "over…"; "界界界界"
	// (8 cells) keeps two wide glyphs and the ellipsis; "ab" fits.
	if got := rowText(b, 0); got != " over…  界界…  ab" {
		t.Fatalf("tab row: %q", got)
	}
	_, _, b = tabsAt(TabSelectOptions{Options: opts, TabWidth: 7}, 12)
	if got := rowText(b, 2); got != " a long de…" {
		t.Fatalf("description: %q", got)
	}
}

func TestTabSelectMouse(t *testing.T) {
	var selected []int
	r, n, _ := tabsAt(TabSelectOptions{Options: fruitOptions(), TabWidth: 8, OnSelect: func(i int, _ SelectOption) { selected = append(selected, i) }}, 24)
	click(r, 17, 0) // "bat"
	if n.SelectedIndex() != 2 || len(selected) != 0 || !n.Focused() {
		t.Fatalf("click: index %d, selects %v, focused %v", n.SelectedIndex(), selected, n.Focused())
	}
	click(r, 20, 1) // its underline row: the same tab, already selected
	if fmt.Sprint(selected) != "[2]" {
		t.Fatalf("click on the selected tab: %v", selected)
	}
	click(r, 2, 2) // the description row is not a tab
	if n.SelectedIndex() != 2 {
		t.Fatalf("description click moved the selection: %d", n.SelectedIndex())
	}
	if e := wheel(r, 2, 0, ScrollUp); !e.PropagationStopped() || n.SelectedIndex() != 1 {
		t.Fatalf("wheel up: index %d", n.SelectedIndex())
	}
	wheel(r, 2, 0, ScrollLeft)
	if e := wheel(r, 2, 0, ScrollUp); e.PropagationStopped() {
		t.Fatal("wheel at the first tab must bubble")
	}
}

// Regression: a click re-centered the view, so the clicked tab moved away
// and a second click on the same spot hit a different tab.
func TestTabSelectClickKeepsTabUnderPointer(t *testing.T) {
	var selected []int
	r, n, _ := tabsAt(TabSelectOptions{Options: numbered(6), TabWidth: 7, OnSelect: func(i int, _ SelectOption) { selected = append(selected, i) }}, 21)
	n.SetSelectedIndex(5) // view shows tabs 3..5
	redraw(r, 21, 3)
	click(r, 2, 0) // tab 3, leftmost
	b := redraw(r, 21, 3)
	if n.SelectedIndex() != 3 || !strings.HasPrefix(rowText(b, 0), "‹item3") {
		t.Fatalf("clicked tab moved: index %d, row %q", n.SelectedIndex(), rowText(b, 0))
	}
	click(r, 2, 0)
	if fmt.Sprint(selected) != "[3]" {
		t.Fatalf("second click on the same tab: %v", selected)
	}
	n.keyHandler(key("left", "\x1b[D")) // keyboard moves re-center again
	b = redraw(r, 21, 3)
	if got := rowText(b, 0); !strings.HasPrefix(got, "‹item1  item2") {
		t.Fatalf("keyboard move after a click: %q", got)
	}

	// Setters re-center too: a click on the leftmost tab 3 pins the view at
	// 3..5; SetSelectOptions and SetTabWidth drop the pin, centering on 3.
	for name, set := range map[string]func(){
		"SetSelectOptions": func() { n.SetSelectOptions(numbered(6)) },
		"SetTabWidth":      func() { n.SetTabWidth(7) },
	} {
		n.SetSelectedIndex(5)
		redraw(r, 21, 3)
		click(r, 2, 0)
		if got := rowText(redraw(r, 21, 3), 0); got != "‹item3  item4  item5" {
			t.Fatalf("%s: pinned view %q", name, got)
		}
		set()
		if got := rowText(redraw(r, 21, 3), 0); got != "‹item2  item3  item4›" || n.SelectedIndex() != 3 {
			t.Fatalf("%s: row %q, index %d", name, got, n.SelectedIndex())
		}
	}
}

func TestTabSelectColors(t *testing.T) {
	bg := MustHex("#202020")
	n := TabSelect(TabSelectOptions{BG: bg})
	if n.tabs.opts.FocusedBG != bg || n.tabs.opts.FocusedFG != MustHex("#FFFFFF") {
		t.Fatalf("focused colors follow BG and FG: %+v", n.tabs.opts)
	}
	if TabSelect(TabSelectOptions{}).tabs.opts.FocusedBG != MustHex("#1a1a1a") {
		t.Fatal("default focused background")
	}
}

func TestTabSelectEdgeCases(t *testing.T) {
	empty := TabSelect(TabSelectOptions{})
	empty.keyHandler(key("right", "\x1b[C"))
	empty.MoveSelection(-2)
	if _, ok := empty.SelectedOption(); ok || empty.SelectCurrent() {
		t.Fatal("empty tabs have an option")
	}
	for _, w := range []int{0, 1, 2, 3, 30} {
		draw(NewNode(Style{}, TabSelect(TabSelectOptions{Options: numbered(4), TabWidth: 1})), w, 3) // must not panic
		draw(NewNode(Style{}, TabSelect(TabSelectOptions{Options: numbered(4)})), w, 1)
	}
	n := TabSelect(TabSelectOptions{Options: fruitOptions(), SelectedIndex: 9})
	if n.SelectedIndex() != 2 {
		t.Fatalf("initial index not clamped: %d", n.SelectedIndex())
	}
}

func FuzzTabSelect(f *testing.F) {
	f.Add(uint8(10), "item", []byte{0, 1, 2, 3}, uint8(20), uint8(6), uint8(3))
	f.Add(uint8(0), "界é", []byte{1, 0, 2}, uint8(1), uint8(1), uint8(1))
	f.Fuzz(func(t *testing.T, count uint8, name string, keys []byte, w, tabWidth, h uint8) {
		var opts []SelectOption
		for i := 0; i < int(count%40); i++ {
			opts = append(opts, SelectOption{Name: fmt.Sprint(name, i), Description: name})
		}
		n := TabSelect(TabSelectOptions{Options: opts, TabWidth: int(tabWidth % 30), WrapSelection: count%2 == 0})
		names := []string{"left", "right", "return", "[", "]"}
		for _, k := range keys {
			n.keyHandler(key(names[int(k)%len(names)], ""))
			if len(opts) > 0 && (n.SelectedIndex() < 0 || n.SelectedIndex() >= len(opts)) {
				t.Fatalf("index %d out of %d", n.SelectedIndex(), len(opts))
			}
		}
		draw(NewNode(Style{}, n), int(w%120), int(h%5))
	})
}
