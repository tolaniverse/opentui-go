package opentui

import "math"

// A port of the flexbox algorithm in Yoga 3.2.1 (facebook/yoga,
// yoga/algorithm/CalculateLayout.cpp, FlexLine.cpp, AbsoluteLayout.cpp,
// PixelGrid.cpp), the layout engine upstream OpenTUI calls through Zig.
//
// Function names follow Yoga's so the two can be read side by side.
// Like Yoga, all math is float32. Left-to-right only, single line
// (no flex-wrap), no aspect ratio, baseline or auto margins.
// testdata/yoga_cases.json checks this file against real Yoga.

// sizingMode says how a node should treat the size its parent offers.
type sizingMode uint8

const (
	stretchFit sizingMode = iota // use exactly this size ("Exactly")
	fitContent                   // use content size, at most this ("AtMost")
	maxContent                   // no limit: use content size ("Undefined")
)

// Physical edges, in Yoga's order for layout.position.
const (
	edgeLeft = iota
	edgeTop
	edgeRight
	edgeBottom
)

// layoutState is a node's working layout data (Yoga's LayoutResults).
type layoutState struct {
	measured      [2]float32 // width, height from the last pass
	dim           [2]float32 // width, height from the last layout pass
	pos           [4]float32 // offset from the parent, per physical edge
	computedBasis float32
}

// ---- axis helpers (FlexDirection.h) ------------------------------------------

func isRow(a FlexDirection) bool { return a == Row || a == RowReverse }

func dimOf(a FlexDirection) int {
	if isRow(a) {
		return 0
	}
	return 1
}

func crossOf(a FlexDirection) FlexDirection {
	if isRow(a) {
		return Column
	}
	return Row
}

func flexStartEdge(a FlexDirection) int {
	switch a {
	case Row:
		return edgeLeft
	case RowReverse:
		return edgeRight
	case Column:
		return edgeTop
	}
	return edgeBottom
}

func flexEndEdge(a FlexDirection) int {
	switch a {
	case Row:
		return edgeRight
	case RowReverse:
		return edgeLeft
	case Column:
		return edgeBottom
	}
	return edgeTop
}

func inlineStartEdge(a FlexDirection) int {
	if isRow(a) {
		return edgeLeft
	}
	return edgeTop
}

func inlineEndEdge(a FlexDirection) int {
	if isRow(a) {
		return edgeRight
	}
	return edgeBottom
}

func needsTrailingPosition(a FlexDirection) bool { return a == RowReverse || a == ColumnReverse }

func maxOrDefined(a, b float32) float32 {
	switch {
	case isNaN(a):
		return b
	case isNaN(b):
		return a
	}
	return max(a, b)
}

func minOrDefined(a, b float32) float32 {
	switch {
	case isNaN(a):
		return b
	case isNaN(b):
		return a
	}
	return min(a, b)
}

func inexactEquals(a, b float64) bool { return math.Abs(a-b) < 0.0001 }

// ---- style accessors ---------------------------------------------------------

func edge(e Edges, which int) float32 {
	switch which {
	case edgeLeft:
		return float32(e.Left)
	case edgeTop:
		return float32(e.Top)
	case edgeRight:
		return float32(e.Right)
	}
	return float32(e.Bottom)
}

func (n *Node) margin(which int) float32  { return edge(n.Style.Margin, which) }
func (n *Node) padding(which int) float32 { return edge(n.Style.Padding, which) }
func (n *Node) borderW(which int) float32 { return edge(n.border, which) }

func (n *Node) marginForAxis(a FlexDirection) float32 {
	return n.margin(inlineStartEdge(a)) + n.margin(inlineEndEdge(a))
}

func (n *Node) borderForAxis(a FlexDirection) float32 {
	return n.borderW(inlineStartEdge(a)) + n.borderW(inlineEndEdge(a))
}

func (n *Node) paddingAndBorderForAxis(a FlexDirection) float32 {
	return n.padding(inlineStartEdge(a)) + n.borderW(inlineStartEdge(a)) +
		n.padding(inlineEndEdge(a)) + n.borderW(inlineEndEdge(a))
}

func (n *Node) flexStartPaddingAndBorder(a FlexDirection) float32 {
	e := flexStartEdge(a)
	return n.padding(e) + n.borderW(e)
}

func (n *Node) flexEndPaddingAndBorder(a FlexDirection) float32 {
	e := flexEndEdge(a)
	return n.padding(e) + n.borderW(e)
}

func (n *Node) dimStyle(d int) (val, lo, hi Dim) {
	if d == 0 {
		return n.Style.Width, n.Style.MinWidth, n.Style.MaxWidth
	}
	return n.Style.Height, n.Style.MinHeight, n.Style.MaxHeight
}

// resolvedDimension resolves Width/Height. When min == max, Yoga uses
// that value instead (Node::processDimensions).
func (n *Node) resolvedDimension(d int, owner float32) float32 {
	v, lo, hi := n.dimStyle(d)
	if hi.kind != dimAuto && hi == lo {
		v = hi
	}
	return v.resolve(owner)
}

func (n *Node) hasDefiniteLength(d int, owner float32) bool {
	v := n.resolvedDimension(d, owner)
	return defined(v) && v >= 0
}

func (n *Node) resolvedMin(d int, owner float32) float32 {
	_, lo, _ := n.dimStyle(d)
	return lo.resolve(owner)
}

func (n *Node) resolvedMax(d int, owner float32) float32 {
	_, _, hi := n.dimStyle(d)
	return hi.resolve(owner)
}

// boundAxisWithinMinAndMax clamps value to the node's min/max on axis.
func (n *Node) boundAxisWithinMinAndMax(a FlexDirection, value, axisSize float32) float32 {
	d := dimOf(a)
	lo, hi := n.resolvedMin(d, axisSize), n.resolvedMax(d, axisSize)
	if defined(hi) && hi >= 0 && value > hi {
		return hi
	}
	if defined(lo) && lo >= 0 && value < lo {
		return lo
	}
	return value
}

// boundAxis is boundAxisWithinMinAndMax that also never goes below
// padding + border.
func (n *Node) boundAxis(a FlexDirection, value, axisSize float32) float32 {
	return maxOrDefined(n.boundAxisWithinMinAndMax(a, value, axisSize), n.paddingAndBorderForAxis(a))
}

func (n *Node) resolveFlexGrow() float32 {
	if n.parent == nil {
		return 0
	}
	return float32(n.Style.FlexGrow)
}

func (n *Node) resolveFlexShrink() float32 {
	if n.parent == nil {
		return 0
	}
	return n.Style.shrink()
}

func (n *Node) isFlexible() bool {
	return n.Style.Position != Absolute && (n.resolveFlexGrow() != 0 || n.resolveFlexShrink() != 0)
}

// resolveChildAlignment: AlignSelf, else the parent's AlignItems
// (whose default, AlignAuto, means Stretch upstream).
func resolveChildAlignment(parent, child *Node) Align {
	a := child.Style.AlignSelf
	if a == AlignAuto {
		a = parent.Style.AlignItems
	}
	if a == AlignAuto {
		a = AlignStretch
	}
	return a
}

func (n *Node) dimensionWithMargin(a FlexDirection) float32 {
	return n.lay.measured[dimOf(a)] + n.marginForAxis(a)
}

func (n *Node) insets(a FlexDirection) (start, end Dim) {
	if isRow(a) {
		return n.Style.Left, n.Style.Right
	}
	return n.Style.Top, n.Style.Bottom
}

// relativePosition is the shift Top/Left (or -Bottom/-Right) applies.
func (n *Node) relativePosition(a FlexDirection, axisSize float32) float32 {
	start, end := n.insets(a)
	if !start.IsAuto() {
		return start.resolve(axisSize)
	}
	if !end.IsAuto() {
		return -end.resolve(axisSize)
	}
	return 0
}

// setPosition resets a child's offsets to its margins plus relative shift
// (Node::setPosition).
func (n *Node) setPosition(ownerWidth, ownerHeight float32) {
	rx := n.relativePosition(Row, ownerWidth)
	ry := n.relativePosition(Column, ownerHeight)
	n.lay.pos[edgeLeft] = n.margin(edgeLeft) + rx
	n.lay.pos[edgeRight] = n.margin(edgeRight) + rx
	n.lay.pos[edgeTop] = n.margin(edgeTop) + ry
	n.lay.pos[edgeBottom] = n.margin(edgeBottom) + ry
}

func zeroOutLayout(n *Node) {
	n.lay = layoutState{}
	n.rect = Rect{}
	for _, c := range n.children {
		zeroOutLayout(c)
	}
}

func measureModeOf(m sizingMode) MeasureMode {
	switch m {
	case stretchFit:
		return MeasureExactly
	case fitContent:
		return MeasureAtMost
	}
	return MeasureUndefined
}

// ---- the algorithm -------------------------------------------------------------

// calculate is calculateLayoutInternal without Yoga's layout cache.
func (n *Node) calculate(availW, availH float32, wm, hm sizingMode, ownerW, ownerH float32, performLayout bool) {
	n.calculateImpl(availW, availH, wm, hm, ownerW, ownerH, performLayout)
	if performLayout {
		n.lay.dim = n.lay.measured
	}
}

func (n *Node) measureWithMeasureFunc(availW, availH float32, wm, hm sizingMode, ownerW, ownerH float32) {
	if wm == maxContent {
		availW = undefined
	}
	if hm == maxContent {
		availH = undefined
	}
	pbRow, pbCol := n.paddingAndBorderForAxis(Row), n.paddingAndBorderForAxis(Column)
	innerW, innerH := availW, availH
	if defined(innerW) {
		innerW = max(0, availW-pbRow)
	}
	if defined(innerH) {
		innerH = max(0, availH-pbCol)
	}
	if wm == stretchFit && hm == stretchFit {
		n.lay.measured[0] = n.boundAxis(Row, availW, ownerW)
		n.lay.measured[1] = n.boundAxis(Column, availH, ownerH)
		return
	}
	mw, mh := n.measure(float64(innerW), measureModeOf(wm), float64(innerH), measureModeOf(hm))
	w, h := availW, availH
	if wm != stretchFit {
		w = float32(mw) + pbRow
	}
	if hm != stretchFit {
		h = float32(mh) + pbCol
	}
	n.lay.measured[0] = n.boundAxis(Row, w, ownerW)
	n.lay.measured[1] = n.boundAxis(Column, h, ownerH)
}

func (n *Node) measureWithoutChildren(availW, availH float32, wm, hm sizingMode, ownerW, ownerH float32) {
	w, h := availW, availH
	if wm != stretchFit {
		w = n.paddingAndBorderForAxis(Row)
	}
	if hm != stretchFit {
		h = n.paddingAndBorderForAxis(Column)
	}
	n.lay.measured[0] = n.boundAxis(Row, w, ownerW)
	n.lay.measured[1] = n.boundAxis(Column, h, ownerH)
}

func (n *Node) measureWithFixedSize(availW, availH float32, wm, hm sizingMode, ownerW, ownerH float32) bool {
	if (defined(availW) && wm == fitContent && availW <= 0) ||
		(defined(availH) && hm == fitContent && availH <= 0) ||
		(wm == stretchFit && hm == stretchFit) {
		w, h := availW, availH
		if isNaN(w) || (wm == fitContent && w < 0) {
			w = 0
		}
		if isNaN(h) || (hm == fitContent && h < 0) {
			h = 0
		}
		n.lay.measured[0] = n.boundAxis(Row, w, ownerW)
		n.lay.measured[1] = n.boundAxis(Column, h, ownerH)
		return true
	}
	return false
}

func (n *Node) availableInnerDimension(d int, avail, pb, owner float32) float32 {
	inner := avail - pb
	if defined(inner) {
		minInner := float32(0)
		if lo := n.resolvedMin(d, owner); defined(lo) {
			minInner = lo - pb
		}
		maxInner := float32(math.MaxFloat32)
		if hi := n.resolvedMax(d, owner); defined(hi) {
			maxInner = hi - pb
		}
		inner = maxOrDefined(minOrDefined(inner, maxInner), minInner)
	}
	return inner
}

// constrainMaxSizeForMode applies a child's max size to the size offered.
func (n *Node) constrainMaxSizeForMode(a FlexDirection, ownerAxisSize float32, mode *sizingMode, size *float32) {
	maxSize := n.resolvedMax(dimOf(a), ownerAxisSize) + n.marginForAxis(a)
	switch *mode {
	case stretchFit, fitContent:
		if defined(maxSize) && !(*size < maxSize) {
			*size = maxSize
		}
	case maxContent:
		if defined(maxSize) {
			*mode = fitContent
			*size = maxSize
		}
	}
}

func (n *Node) computeFlexBasisForChild(child *Node, width float32, wMode sizingMode, height, ownerW, ownerH float32, hMode sizingMode) {
	main := n.Style.Direction
	mainRow := isRow(main)
	mainSize, mainOwner := height, ownerH
	if mainRow {
		mainSize, mainOwner = width, ownerW
	}
	basis := child.Style.FlexBasis.resolve(mainOwner)
	rowDef := child.hasDefiniteLength(0, ownerW)
	colDef := child.hasDefiniteLength(1, ownerH)

	switch {
	case defined(basis) && defined(mainSize):
		child.lay.computedBasis = maxOrDefined(basis, child.paddingAndBorderForAxis(main))
	case mainRow && rowDef:
		child.lay.computedBasis = maxOrDefined(child.resolvedDimension(0, ownerW), child.paddingAndBorderForAxis(Row))
	case !mainRow && colDef:
		child.lay.computedBasis = maxOrDefined(child.resolvedDimension(1, ownerH), child.paddingAndBorderForAxis(Column))
	default:
		cw, ch := undefined, undefined
		cwm, chm := maxContent, maxContent
		marginRow, marginCol := child.marginForAxis(Row), child.marginForAxis(Column)
		if rowDef {
			cw, cwm = child.resolvedDimension(0, ownerW)+marginRow, stretchFit
		}
		if colDef {
			ch, chm = child.resolvedDimension(1, ownerH)+marginCol, stretchFit
		}
		// Browsers measure with "at most the available space" unless the
		// container scrolls; there is no scrolling overflow here (yet).
		if isNaN(cw) && defined(width) {
			cw, cwm = width, fitContent
		}
		if isNaN(ch) && defined(height) {
			ch, chm = height, fitContent
		}
		stretch := resolveChildAlignment(n, child) == AlignStretch
		if !mainRow && !rowDef && defined(width) && wMode == stretchFit && stretch && cwm != stretchFit {
			cw, cwm = width, stretchFit
		}
		if mainRow && !colDef && defined(height) && hMode == stretchFit && stretch && chm != stretchFit {
			ch, chm = height, stretchFit
		}
		child.constrainMaxSizeForMode(Row, ownerW, &cwm, &cw)
		child.constrainMaxSizeForMode(Column, ownerH, &chm, &ch)
		child.calculate(cw, ch, cwm, chm, ownerW, ownerH, false)
		child.lay.computedBasis = maxOrDefined(child.lay.measured[dimOf(main)], child.paddingAndBorderForAxis(main))
	}
}

func (n *Node) computeFlexBasisForChildren(innerW, innerH float32, wMode, hMode sizingMode, performLayout bool) float32 {
	main := n.Style.Direction
	mainMode := hMode
	if isRow(main) {
		mainMode = wMode
	}
	// A sole flexible child (grow and shrink both non-zero) in a fixed-size
	// container starts from a basis of 0 — Yoga's shortcut.
	var single *Node
	if mainMode == stretchFit {
		for _, c := range n.children {
			if !c.isFlexible() {
				continue
			}
			if single != nil || inexactEquals(float64(c.resolveFlexGrow()), 0) || inexactEquals(float64(c.resolveFlexShrink()), 0) {
				single = nil
				break
			}
			single = c
		}
	}
	var total float32
	for _, c := range n.children {
		if c.Hidden {
			zeroOutLayout(c)
			continue
		}
		if performLayout {
			c.setPosition(innerW, innerH)
		}
		if c.Style.Position == Absolute {
			continue
		}
		if c == single {
			c.lay.computedBasis = 0
		} else {
			n.computeFlexBasisForChild(c, innerW, wMode, innerH, innerW, innerH, hMode)
		}
		total += c.lay.computedBasis + c.marginForAxis(main)
	}
	return total
}

// flexLine is one line of in-flow children (Yoga's FlexLine).
type flexLine struct {
	items        []*Node
	sizeConsumed float32
	totalGrow    float32
	totalShrink  float32 // sum of -shrink × basis
	remaining    float32
	mainDim      float32
	crossDim     float32
}

func (n *Node) calculateFlexLine(mainOwnerSize, innerMain float32) flexLine {
	var l flexLine
	main := n.Style.Direction
	gap := float32(n.Style.Gap)
	for _, c := range n.children {
		if c.Hidden || c.Style.Position == Absolute {
			continue
		}
		lead := gap
		if len(l.items) == 0 {
			lead = 0
		}
		l.sizeConsumed += c.boundAxisWithinMinAndMax(main, c.lay.computedBasis, mainOwnerSize) + c.marginForAxis(main) + lead
		if c.isFlexible() {
			l.totalGrow += c.resolveFlexGrow()
			l.totalShrink += -c.resolveFlexShrink() * c.lay.computedBasis
		}
		l.items = append(l.items, c)
	}
	if l.totalGrow > 0 && l.totalGrow < 1 {
		l.totalGrow = 1
	}
	if l.totalShrink > 0 && l.totalShrink < 1 {
		l.totalShrink = 1
	}
	return l
}

func (n *Node) distributeFreeSpaceFirstPass(l *flexLine, mainOwnerSize, innerMain float32) {
	main := n.Style.Direction
	var delta float32
	for _, c := range l.items {
		basis := c.boundAxisWithinMinAndMax(main, c.lay.computedBasis, mainOwnerSize)
		if l.remaining < 0 {
			scaled := -c.resolveFlexShrink() * basis
			if defined(scaled) && scaled != 0 {
				base := basis + l.remaining/l.totalShrink*scaled
				bound := c.boundAxis(main, base, innerMain)
				// Yoga's C++/Wasm arithmetic lands exactly on the bound in cases
				// where Go's float32 expression can differ by one ULP. Treat
				// values within Yoga's own 0.0001 epsilon as equal; otherwise a
				// false freeze changes every later shrink calculation.
				if defined(base) && defined(bound) && !inexactEquals(float64(base), float64(bound)) {
					delta += bound - basis
					l.totalShrink -= -c.resolveFlexShrink() * c.lay.computedBasis
				}
			}
		} else if defined(l.remaining) && l.remaining > 0 {
			grow := c.resolveFlexGrow()
			if defined(grow) && grow != 0 {
				base := basis + l.remaining/l.totalGrow*grow
				bound := c.boundAxis(main, base, innerMain)
				if defined(base) && defined(bound) && base != bound {
					delta += bound - basis
					l.totalGrow -= grow
				}
			}
		}
	}
	l.remaining -= delta
}

func (n *Node) distributeFreeSpaceSecondPass(l *flexLine, mainOwnerSize, innerMain, innerCross, innerW, innerH float32, crossMode sizingMode, performLayout bool) float32 {
	main := n.Style.Direction
	cross := crossOf(main)
	mainRow := isRow(main)
	var delta float32
	for _, c := range l.items {
		basis := c.boundAxisWithinMinAndMax(main, c.lay.computedBasis, mainOwnerSize)
		updated := basis
		if defined(l.remaining) && l.remaining < 0 {
			scaled := -c.resolveFlexShrink() * basis
			if scaled != 0 {
				var size float32
				// OpenTUI's Yoga patch (yoga-flex-shrink-residue, backport of
				// facebook/yoga#1974 and #2044): after the first pass, float
				// cancellation can leave a near-zero total instead of 0, and
				// dividing by it explodes. Treat |total| < 1e-6, or any
				// positive total (never real with shrink ≥ 0), as zero.
				if defined(l.totalShrink) && (abs32(l.totalShrink) < 1e-6 || l.totalShrink > 0) {
					size = basis + scaled
				} else {
					size = basis + (l.remaining/l.totalShrink)*scaled
				}
				updated = c.boundAxis(main, size, innerMain)
			}
		} else if defined(l.remaining) && l.remaining > 0 {
			if grow := c.resolveFlexGrow(); defined(grow) && grow != 0 {
				updated = c.boundAxis(main, basis+l.remaining/l.totalGrow*grow, innerMain)
			}
		}
		delta += updated - basis

		marginMain, marginCross := c.marginForAxis(main), c.marginForAxis(cross)
		childMain := updated + marginMain
		mainMode := stretchFit
		var childCross float32
		var crossChildMode sizingMode
		crossDefinite := c.hasDefiniteLength(dimOf(cross), innerCross)
		stretch := resolveChildAlignment(n, c) == AlignStretch
		switch {
		case defined(innerCross) && !crossDefinite && crossMode == stretchFit && stretch:
			childCross, crossChildMode = innerCross, stretchFit
		case !crossDefinite:
			childCross, crossChildMode = innerCross, fitContent
			if isNaN(childCross) {
				crossChildMode = maxContent
			}
		default:
			childCross = c.resolvedDimension(dimOf(cross), innerCross) + marginCross
			crossDim, _, _ := c.dimStyle(dimOf(cross))
			loose := crossDim.kind == dimPercent && crossMode != stretchFit
			crossChildMode = stretchFit
			if isNaN(childCross) || loose {
				crossChildMode = maxContent
			}
		}
		c.constrainMaxSizeForMode(main, innerMain, &mainMode, &childMain)
		c.constrainMaxSizeForMode(cross, innerCross, &crossChildMode, &childCross)

		requiresStretch := !crossDefinite && stretch
		cw, ch, cwm, chm := childCross, childMain, crossChildMode, mainMode
		if mainRow {
			cw, ch, cwm, chm = childMain, childCross, mainMode, crossChildMode
		}
		c.calculate(cw, ch, cwm, chm, innerW, innerH, performLayout && !requiresStretch)
	}
	return delta
}

func (n *Node) resolveFlexibleLength(l *flexLine, mainOwnerSize, innerMain, innerCross, innerW, innerH float32, crossMode sizingMode, performLayout bool) {
	original := l.remaining
	n.distributeFreeSpaceFirstPass(l, mainOwnerSize, innerMain)
	distributed := n.distributeFreeSpaceSecondPass(l, mainOwnerSize, innerMain, innerCross, innerW, innerH, crossMode, performLayout)
	l.remaining = original - distributed
}

func (n *Node) justifyMainAxis(l *flexLine, mainMode, crossMode sizingMode, mainOwnerSize, innerMain, innerCross float32, performLayout bool) {
	main := n.Style.Direction
	cross := crossOf(main)
	leadPB, trailPB := n.flexStartPaddingAndBorder(main), n.flexEndPaddingAndBorder(main)
	gap := float32(n.Style.Gap)

	if mainMode == fitContent && l.remaining > 0 {
		if lo := n.resolvedMin(dimOf(main), mainOwnerSize); defined(lo) {
			minAvail := lo - leadPB - trailPB
			occupied := innerMain - l.remaining
			l.remaining = maxOrDefined(0, minAvail-occupied)
		} else {
			l.remaining = 0
		}
	}

	var leading float32
	between := gap
	j := n.Style.Justify
	if !(l.remaining >= 0) { // fallback alignment when overflowing
		switch j {
		case JustifySpaceBetween, JustifySpaceAround, JustifySpaceEvenly:
			j = JustifyStart
		}
	}
	count := float32(len(l.items))
	switch j {
	case JustifyCenter:
		leading = l.remaining / 2
	case JustifyEnd:
		leading = l.remaining
	case JustifySpaceBetween:
		if len(l.items) > 1 {
			between += l.remaining / (count - 1)
		}
	case JustifySpaceEvenly:
		leading = l.remaining / (count + 1)
		between += leading
	case JustifySpaceAround:
		leading = 0.5 * l.remaining / count
		between += leading * 2
	}

	l.mainDim = leadPB + leading
	l.crossDim = 0
	start := flexStartEdge(main)
	for i, c := range l.items {
		if performLayout {
			c.lay.pos[start] += l.mainDim
		}
		if i != len(l.items)-1 {
			l.mainDim += between
		}
		if !performLayout && crossMode == stretchFit {
			l.mainDim += c.marginForAxis(main) + c.lay.computedBasis
			l.crossDim = innerCross
		} else {
			l.mainDim += c.dimensionWithMargin(main)
			l.crossDim = maxOrDefined(l.crossDim, c.dimensionWithMargin(cross))
		}
	}
	l.mainDim += trailPB
}

// calculateImpl is Yoga's calculateLayoutImpl: measure (performLayout =
// false) or lay out (true) n inside the space offered.
func (n *Node) calculateImpl(availW, availH float32, wMode, hMode sizingMode, ownerW, ownerH float32, performLayout bool) {
	marginRow, marginCol := n.marginForAxis(Row), n.marginForAxis(Column)

	if n.measure != nil {
		n.measureWithMeasureFunc(availW-marginRow, availH-marginCol, wMode, hMode, ownerW, ownerH)
		return
	}
	if len(n.children) == 0 {
		n.measureWithoutChildren(availW-marginRow, availH-marginCol, wMode, hMode, ownerW, ownerH)
		return
	}
	if !performLayout && n.measureWithFixedSize(availW-marginRow, availH-marginCol, wMode, hMode, ownerW, ownerH) {
		return
	}

	main := n.Style.Direction
	cross := crossOf(main)
	mainRow := isRow(main)
	mainOwnerSize, crossOwnerSize := ownerH, ownerW
	mainMode, crossMode := hMode, wMode
	if mainRow {
		mainOwnerSize, crossOwnerSize = ownerW, ownerH
		mainMode, crossMode = wMode, hMode
	}
	pbMain, pbCross := n.paddingAndBorderForAxis(main), n.paddingAndBorderForAxis(cross)
	leadPBCross := n.flexStartPaddingAndBorder(cross)
	pbRow, pbCol := n.paddingAndBorderForAxis(Row), n.paddingAndBorderForAxis(Column)

	innerW := n.availableInnerDimension(0, availW-marginRow, pbRow, ownerW)
	innerH := n.availableInnerDimension(1, availH-marginCol, pbCol, ownerH)
	innerMain, innerCross := innerH, innerW
	if mainRow {
		innerMain, innerCross = innerW, innerH
	}

	// STEP 3: flex basis of every child.
	n.computeFlexBasisForChildren(innerW, innerH, wMode, hMode, performLayout)

	// STEP 4: collect the line (no wrapping: one line).
	l := n.calculateFlexLine(mainOwnerSize, innerMain)
	canSkipFlex := !performLayout && crossMode == stretchFit

	// STEP 5: when the main size is not fixed, decide how much room flexing gets.
	sizeBasedOnContent := false
	if mainMode != stretchFit {
		d := dimOf(main)
		minInner := n.resolvedMin(d, mainOwnerSize) - pbMain
		maxInner := n.resolvedMax(d, mainOwnerSize) - pbMain
		switch {
		case defined(minInner) && l.sizeConsumed < minInner:
			innerMain = minInner
		case defined(maxInner) && l.sizeConsumed > maxInner:
			innerMain = maxInner
		default:
			if (defined(l.totalGrow) && l.totalGrow == 0) || n.resolveFlexGrow() == 0 {
				innerMain = l.sizeConsumed
			}
			sizeBasedOnContent = true
		}
	}
	if !sizeBasedOnContent && defined(innerMain) {
		l.remaining = innerMain - l.sizeConsumed
	} else if l.sizeConsumed < 0 {
		l.remaining = -l.sizeConsumed
	}

	// STEP 6–7: grow/shrink, then place along the main axis.
	if !canSkipFlex {
		n.resolveFlexibleLength(&l, mainOwnerSize, innerMain, innerCross, innerW, innerH, crossMode, performLayout)
	}
	n.justifyMainAxis(&l, mainMode, crossMode, mainOwnerSize, innerMain, innerCross, performLayout)

	containerCross := innerCross
	if crossMode != stretchFit {
		containerCross = n.boundAxis(cross, l.crossDim+pbCross, crossOwnerSize) - pbCross
	}
	if crossMode == stretchFit {
		l.crossDim = innerCross
	}
	l.crossDim = n.boundAxis(cross, l.crossDim+pbCross, crossOwnerSize) - pbCross

	// STEP 8: cross-axis alignment (and the stretch re-layout).
	if performLayout {
		crossStart := flexStartEdge(cross)
		for _, c := range l.items {
			leading := leadPBCross
			align := resolveChildAlignment(n, c)
			if align == AlignStretch {
				if !c.hasDefiniteLength(dimOf(cross), innerCross) {
					childMain := c.lay.measured[dimOf(main)] + c.marginForAxis(main)
					childCross := l.crossDim
					mm, cm := stretchFit, stretchFit
					c.constrainMaxSizeForMode(main, innerMain, &mm, &childMain)
					c.constrainMaxSizeForMode(cross, innerCross, &cm, &childCross)
					cw, ch := childCross, childMain
					if mainRow {
						cw, ch = childMain, childCross
					}
					cwm, chm := stretchFit, stretchFit
					if isNaN(cw) {
						cwm = maxContent
					}
					if isNaN(ch) {
						chm = maxContent
					}
					c.calculate(cw, ch, cwm, chm, innerW, innerH, true)
				}
			} else {
				remaining := containerCross - c.dimensionWithMargin(cross)
				switch align {
				case AlignCenter:
					leading += remaining / 2
				case AlignEnd:
					leading += remaining
				}
			}
			c.lay.pos[crossStart] += leading
		}
	}

	// STEP 9: this node's own size.
	n.lay.measured[0] = n.boundAxis(Row, availW-marginRow, ownerW)
	n.lay.measured[1] = n.boundAxis(Column, availH-marginCol, ownerH)
	if mainMode != stretchFit {
		n.lay.measured[dimOf(main)] = n.boundAxis(main, l.mainDim, mainOwnerSize)
	}
	if crossMode != stretchFit {
		n.lay.measured[dimOf(cross)] = n.boundAxis(cross, l.crossDim+pbCross, crossOwnerSize)
	}

	if performLayout {
		// STEP 10: reversed axes were placed from the far edge; convert.
		if needsTrailingPosition(main) {
			for _, c := range n.children {
				if !c.Hidden && c.Style.Position != Absolute {
					setChildTrailingPosition(n, c, main)
				}
			}
		}
		// STEP 11: absolutely positioned children.
		wm := crossMode
		if mainRow {
			wm = mainMode
		}
		n.layoutAbsoluteDescendants(wm)
	}
}

func setChildTrailingPosition(n, c *Node, a FlexDirection) {
	size := c.lay.measured[dimOf(a)]
	c.lay.pos[flexEndEdge(a)] = n.lay.measured[dimOf(a)] - size - c.lay.pos[flexStartEdge(a)]
}

// ---- absolute layout (AbsoluteLayout.cpp) -------------------------------------

func oppositeEdge(pos float32, a FlexDirection, parent, c *Node) float32 {
	return parent.lay.measured[dimOf(a)] - c.lay.measured[dimOf(a)] - pos
}

func (n *Node) layoutAbsoluteDescendants(widthMode sizingMode) {
	cbW := n.lay.measured[0] - n.borderForAxis(Row)
	cbH := n.lay.measured[1] - n.borderForAxis(Column)
	main := n.Style.Direction
	cross := crossOf(main)
	for _, c := range n.children {
		if c.Hidden || c.Style.Position != Absolute {
			continue
		}
		n.layoutAbsoluteChild(c, cbW, cbH, widthMode)
		if needsTrailingPosition(main) {
			setChildTrailingPosition(n, c, main)
		}
		if needsTrailingPosition(cross) {
			setChildTrailingPosition(n, c, cross)
		}
	}
}

func (n *Node) layoutAbsoluteChild(c *Node, cbW, cbH float32, widthMode sizingMode) {
	main := n.Style.Direction
	mainRow := isRow(main)
	cw, ch := undefined, undefined
	marginRow, marginCol := c.marginForAxis(Row), c.marginForAxis(Column)

	if c.hasDefiniteLength(0, cbW) {
		cw = c.resolvedDimension(0, cbW) + marginRow
	} else if l, r := c.Style.Left, c.Style.Right; !l.IsAuto() && !r.IsAuto() {
		cw = n.lay.measured[0] - n.borderForAxis(Row) - (l.resolve(cbW) + r.resolve(cbW))
		cw = c.boundAxis(Row, cw, cbW)
	}
	if c.hasDefiniteLength(1, cbH) {
		ch = c.resolvedDimension(1, cbH) + marginCol
	} else if t, b := c.Style.Top, c.Style.Bottom; !t.IsAuto() && !b.IsAuto() {
		ch = n.lay.measured[1] - n.borderForAxis(Column) - (t.resolve(cbH) + b.resolve(cbH))
		ch = c.boundAxis(Column, ch, cbH)
	}

	if isNaN(cw) || isNaN(ch) {
		cwm, chm := stretchFit, stretchFit
		if isNaN(cw) {
			cwm = maxContent
		}
		if isNaN(ch) {
			chm = maxContent
		}
		if !mainRow && isNaN(cw) && widthMode != maxContent && defined(cbW) && cbW > 0 {
			cw, cwm = cbW, fitContent
		}
		c.calculate(cw, ch, cwm, chm, cbW, cbH, false)
		cw = c.lay.measured[0] + marginRow
		ch = c.lay.measured[1] + marginCol
	}
	c.calculate(cw, ch, stretchFit, stretchFit, cbW, cbH, true)

	n.positionAbsoluteChild(c, main, true, cbW, cbH)
	n.positionAbsoluteChild(c, crossOf(main), false, cbW, cbH)
}

func (n *Node) positionAbsoluteChild(c *Node, a FlexDirection, isMain bool, cbW, cbH float32) {
	cbSize := cbH
	if isRow(a) {
		cbSize = cbW
	}
	start, end := c.insets(a)
	reversed := inlineStartEdge(a) != flexStartEdge(a)
	switch {
	case !start.IsAuto():
		pos := start.resolve(cbSize) + n.borderW(inlineStartEdge(a)) + c.margin(inlineStartEdge(a))
		if reversed {
			pos = oppositeEdge(pos, a, n, c)
		}
		c.lay.pos[flexStartEdge(a)] = pos
	case !end.IsAuto():
		pos := n.lay.measured[dimOf(a)] - c.lay.measured[dimOf(a)] -
			n.borderW(inlineEndEdge(a)) - c.margin(inlineEndEdge(a)) - end.resolve(cbSize)
		if reversed {
			pos = oppositeEdge(pos, a, n, c)
		}
		c.lay.pos[flexStartEdge(a)] = pos
	default:
		var where Justify // reuse Justify as start / center / end
		if isMain {
			switch n.Style.Justify {
			case JustifyEnd:
				where = JustifyEnd
			case JustifyCenter, JustifySpaceAround, JustifySpaceEvenly:
				where = JustifyCenter
			}
		} else {
			switch resolveChildAlignment(n, c) {
			case AlignEnd:
				where = JustifyEnd
			case AlignCenter:
				where = JustifyCenter
			}
		}
		fs, fe := flexStartEdge(a), flexEndEdge(a)
		switch where {
		case JustifyStart:
			c.lay.pos[fs] = c.margin(fs) + n.borderW(fs) + n.padding(fs)
		case JustifyEnd:
			endPos := n.borderW(fe) + c.margin(fe) + n.padding(fe)
			c.lay.pos[fs] = oppositeEdge(endPos, a, n, c)
		case JustifyCenter:
			content := n.lay.measured[dimOf(a)] - n.borderW(fs) - n.borderW(fe) - n.padding(fs) - n.padding(fe)
			outer := c.lay.measured[dimOf(a)] + c.marginForAxis(a)
			c.lay.pos[fs] = (content-outer)/2 + n.borderW(fs) + c.margin(fs) + n.padding(fs)
		}
	}
}

// ---- rounding (PixelGrid.cpp) ----------------------------------------------------

// roundValueToPixelGrid rounds half up, snapping values within 0.0001 of
// a whole cell, with a point scale factor of 1 (one cell = one point).
func roundValueToPixelGrid(v float64) float64 {
	frac := math.Mod(v, 1)
	if frac < 0 {
		frac++
	}
	switch {
	case inexactEquals(frac, 0):
		return v - frac
	case inexactEquals(frac, 1):
		return v - frac + 1
	case frac > 0.5 || inexactEquals(frac, 0.5):
		return v - frac + 1
	}
	return v - frac
}

// roundToGrid turns layout results into whole cells. Upstream OpenTUI
// patches Yoga to round each node's position from its absolute edges
// (vendor/zig-deps.md, "yoga-absolute-edge-rounding"), so neighbours
// never overlap or leave gaps and children never poke out of parents.
func (n *Node) roundToGrid(absLeft, absTop float64) {
	if n.Hidden {
		n.rect = Rect{}
		return
	}
	left := absLeft + float64(n.lay.pos[edgeLeft])
	top := absTop + float64(n.lay.pos[edgeTop])
	x, y := roundValueToPixelGrid(left), roundValueToPixelGrid(top)
	r := roundValueToPixelGrid(left + float64(n.lay.dim[0]))
	b := roundValueToPixelGrid(top + float64(n.lay.dim[1]))
	n.rect = Rect{X: int(x), Y: int(y), W: int(r - x), H: int(b - y)}
	for _, c := range n.children {
		c.roundToGrid(left, top)
	}
}

func abs32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
