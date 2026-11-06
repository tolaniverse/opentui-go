package opentui

import "testing"

func rect(x, y, w, h int) Rect { return Rect{x, y, w, h} }

func expectRect(t *testing.T, name string, n *Node, want Rect) {
	t.Helper()
	if got := n.Rect(); got != want {
		t.Errorf("%s: got %+v, want %+v", name, got, want)
	}
}

func TestColumnDefaultStretch(t *testing.T) {
	a := NewNode(Style{Height: Cells(3)})
	b := NewNode(Style{Height: Cells(2)})
	root := NewNode(Style{}, a, b)
	ComputeLayout(root, 20, 10)
	expectRect(t, "root", root, rect(0, 0, 20, 10))
	expectRect(t, "a", a, rect(0, 0, 20, 3)) // column + stretch: full width
	expectRect(t, "b", b, rect(0, 3, 20, 2))
}

func TestRowFixedAndGrow(t *testing.T) {
	side := NewNode(Style{Width: Cells(10)})
	main := NewNode(Style{FlexGrow: 1})
	root := NewNode(Style{Direction: Row}, side, main)
	ComputeLayout(root, 50, 8)
	expectRect(t, "side", side, rect(0, 0, 10, 8))
	expectRect(t, "main", main, rect(10, 0, 40, 8))
}

func TestGrowRatioAndRounding(t *testing.T) {
	// 10 cells split 1:2 → 3.33 and 6.67. Rounding the edges (0, 3.33→3,
	// 10) keeps the total exact: widths 3 and 7, no gap, no overlap.
	a := NewNode(Style{FlexGrow: 1})
	b := NewNode(Style{FlexGrow: 2})
	root := NewNode(Style{Direction: Row}, a, b)
	ComputeLayout(root, 10, 1)
	expectRect(t, "a", a, rect(0, 0, 3, 1))
	expectRect(t, "b", b, rect(3, 0, 7, 1))
}

func TestPaddingBorderGap(t *testing.T) {
	a := NewNode(Style{FlexGrow: 1})
	b := NewNode(Style{FlexGrow: 1})
	box := Box(BoxOptions{Border: BorderAll, Style: Style{Direction: Row, Padding: XY(1, 0), Gap: 2, Height: Cells(5)}}, a, b)
	root := NewNode(Style{}, box)
	ComputeLayout(root, 20, 5)
	// inner width = 20 - 2 border - 2 padding = 16; minus gap 2 = 14 → 7 each
	expectRect(t, "a", a, rect(2, 1, 7, 3))
	expectRect(t, "b", b, rect(11, 1, 7, 3))
}

func TestPercentAndMinMax(t *testing.T) {
	a := NewNode(Style{Width: Pct(25)})
	b := NewNode(Style{FlexGrow: 1, MaxWidth: Cells(30)})
	root := NewNode(Style{Direction: Row}, a, b)
	ComputeLayout(root, 100, 4)
	expectRect(t, "a", a, rect(0, 0, 25, 4))
	expectRect(t, "b", b, rect(25, 0, 30, 4)) // capped by MaxWidth
}

func TestShrinkDefaults(t *testing.T) {
	// fixed: Width in cells → shrink 0 (upstream rule). auto: shrink 1.
	fixed := NewNode(Style{Width: Cells(8)})
	auto := NewNode(Style{FlexBasis: Cells(10)})
	root := NewNode(Style{Direction: Row}, fixed, auto)
	ComputeLayout(root, 12, 1)
	expectRect(t, "fixed", fixed, rect(0, 0, 8, 1))
	expectRect(t, "auto", auto, rect(8, 0, 4, 1)) // took all the overflow
}

func TestShrinkProportionalToBasis(t *testing.T) {
	a := NewNode(Style{FlexBasis: Cells(20)})
	b := NewNode(Style{FlexBasis: Cells(10)})
	root := NewNode(Style{Direction: Row}, a, b)
	ComputeLayout(root, 24, 1) // overflow 6, shared 2:1 by basis → -4, -2
	expectRect(t, "a", a, rect(0, 0, 16, 1))
	expectRect(t, "b", b, rect(16, 0, 8, 1))
}

func TestGrowFreezesAtMax(t *testing.T) {
	a := NewNode(Style{FlexGrow: 1, MaxWidth: Cells(5)})
	b := NewNode(Style{FlexGrow: 1})
	root := NewNode(Style{Direction: Row}, a, b)
	ComputeLayout(root, 30, 1)
	expectRect(t, "a", a, rect(0, 0, 5, 1))
	expectRect(t, "b", b, rect(5, 0, 25, 1)) // gets what a could not take
}

func TestJustify(t *testing.T) {
	cases := []struct {
		j    Justify
		want []int // x of each 2-wide child in 12 cells
	}{
		{JustifyStart, []int{0, 2, 4}},
		{JustifyCenter, []int{3, 5, 7}},
		{JustifyEnd, []int{6, 8, 10}},
		{JustifySpaceBetween, []int{0, 5, 10}},
		{JustifySpaceAround, []int{1, 5, 9}},
		{JustifySpaceEvenly, []int{2, 5, 9}}, // 1.5, 5, 8.5 → round half away from zero
	}
	for _, tc := range cases {
		var kids []*Node
		for range 3 {
			kids = append(kids, NewNode(Style{Width: Cells(2)}))
		}
		root := NewNode(Style{Direction: Row, Justify: tc.j}, kids...)
		ComputeLayout(root, 12, 1)
		for i, k := range kids {
			if k.Rect().X != tc.want[i] {
				t.Errorf("justify %d child %d: x=%d want %d", tc.j, i, k.Rect().X, tc.want[i])
			}
		}
	}
}

func TestAlign(t *testing.T) {
	start := NewNode(Style{Height: Cells(2), AlignSelf: AlignStart})
	center := NewNode(Style{Height: Cells(2), AlignSelf: AlignCenter})
	end := NewNode(Style{Height: Cells(2), AlignSelf: AlignEnd})
	stretch := NewNode(Style{})
	root := NewNode(Style{Direction: Row}, start, center, end, stretch)
	ComputeLayout(root, 8, 10)
	if start.Rect().Y != 0 || center.Rect().Y != 4 || end.Rect().Y != 8 {
		t.Errorf("ys: %d %d %d", start.Rect().Y, center.Rect().Y, end.Rect().Y)
	}
	if stretch.Rect().H != 10 {
		t.Errorf("stretch h = %d", stretch.Rect().H)
	}
}

func TestReverse(t *testing.T) {
	a := NewNode(Style{Width: Cells(3)})
	b := NewNode(Style{Width: Cells(2)})
	root := NewNode(Style{Direction: RowReverse}, a, b)
	ComputeLayout(root, 10, 1)
	expectRect(t, "a", a, rect(7, 0, 3, 1)) // first child at the far end
	expectRect(t, "b", b, rect(5, 0, 2, 1))
}

func TestMargins(t *testing.T) {
	a := NewNode(Style{Height: Cells(1), Margin: Edges{Top: 1, Left: 2, Right: 3, Bottom: 1}})
	b := NewNode(Style{Height: Cells(1)})
	root := NewNode(Style{}, a, b)
	ComputeLayout(root, 20, 10)
	expectRect(t, "a", a, rect(2, 1, 15, 1)) // stretch minus margins
	expectRect(t, "b", b, rect(0, 3, 20, 1))
}

func TestHiddenTakesNoSpace(t *testing.T) {
	a := NewNode(Style{Height: Cells(4)})
	a.Hidden = true
	b := NewNode(Style{Height: Cells(1)})
	root := NewNode(Style{}, a, b)
	ComputeLayout(root, 5, 5)
	expectRect(t, "b", b, rect(0, 0, 5, 1))
}

func TestAbsolute(t *testing.T) {
	flow := NewNode(Style{Height: Cells(2)})
	tl := NewNode(Style{Position: Absolute, Left: Cells(1), Top: Cells(1), Width: Cells(4), Height: Cells(2)})
	br := NewNode(Style{Position: Absolute, Right: Cells(0), Bottom: Cells(0), Width: Cells(3), Height: Cells(1)})
	fill := NewNode(Style{Position: Absolute, Left: Cells(2), Right: Cells(2), Top: Cells(0), Height: Cells(1)})
	box := Box(BoxOptions{Border: BorderAll, Style: Style{FlexGrow: 1}}, flow, tl, br, fill) // grow: fill the 10 rows
	root := NewNode(Style{}, box)
	ComputeLayout(root, 20, 10)
	expectRect(t, "flow", flow, rect(1, 1, 18, 2)) // absolute siblings take no space
	expectRect(t, "tl", tl, rect(2, 2, 4, 2))      // inset from inside the border
	expectRect(t, "br", br, rect(16, 8, 3, 1))
	expectRect(t, "fill", fill, rect(3, 1, 14, 1)) // left+right decide width
}

func TestAutoSizeFromContent(t *testing.T) {
	txt := Text("hello\nworld!", TextOptions{})
	box := Box(BoxOptions{Border: BorderAll, Style: Style{AlignSelf: AlignStart}}, txt)
	root := NewNode(Style{}, box)
	ComputeLayout(root, 40, 20)
	expectRect(t, "box", box, rect(0, 0, 8, 4)) // 6×2 text + border
	expectRect(t, "txt", txt, rect(1, 1, 6, 2))
}

func TestTreeOps(t *testing.T) {
	a, b := NewNode(Style{}), NewNode(Style{})
	a.ID = "a"
	p1 := NewNode(Style{}, a)
	p2 := NewNode(Style{})
	p2.Add(a) // moves a
	if len(p1.Children()) != 0 || a.Parent() != p2 {
		t.Fatal("Add must move a node from its old parent")
	}
	if p2.Find("a") != a || p2.Find("zzz") != nil {
		t.Error("Find")
	}
	if p2.Remove(b) {
		t.Error("removing a non-child must return false")
	}
	defer func() {
		if recover() == nil {
			t.Error("adding an ancestor must panic")
		}
	}()
	a.Add(p2)
}
