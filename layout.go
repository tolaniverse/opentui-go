package opentui

import "math"

// Public layout vocabulary: dimensions, edges, enums and Style.
// The algorithm itself is in yoga.go.

// ---- Dimensions ------------------------------------------------------------

type dimKind uint8

const (
	dimAuto dimKind = iota
	dimCells
	dimPercent
)

// Dim is a length: Auto (the zero value), a number of cells, or a percentage
// of the parent's inner size.
type Dim struct {
	kind dimKind
	v    float32
}

// Auto lets the layout decide (from content, stretch or flex).
var Auto = Dim{}

// Cells is a fixed length in terminal cells.
func Cells(n int) Dim { return Dim{dimCells, float32(n)} }

// Pct is a percentage (0–100) of the parent's inner size.
func Pct(p float64) Dim { return Dim{dimPercent, float32(p)} }

// IsAuto reports whether d is Auto.
func (d Dim) IsAuto() bool { return d.kind == dimAuto }

// resolve turns d into cells. A percentage of an unknown (NaN) size, and
// Auto, resolve to NaN ("undefined").
func (d Dim) resolve(parent float32) float32 {
	switch d.kind {
	case dimCells:
		return d.v
	case dimPercent:
		return parent * d.v / 100 // NaN parent → NaN
	}
	return undefined
}

// Edges holds a value per side, in cells (padding, margin, border widths).
type Edges struct{ Top, Right, Bottom, Left int }

// All returns the same value on every side.
func All(n int) Edges { return Edges{n, n, n, n} }

// XY returns x on the left/right and y on the top/bottom.
func XY(x, y int) Edges { return Edges{Top: y, Right: x, Bottom: y, Left: x} }

// ---- Enums (zero value = upstream default) ----------------------------------

// FlexDirection is the main axis children are laid out along.
type FlexDirection uint8

const (
	Column FlexDirection = iota // top to bottom (upstream default)
	Row                         // left to right
	ColumnReverse
	RowReverse
)

// Justify distributes leftover space along the main axis.
type Justify uint8

const (
	JustifyStart Justify = iota
	JustifyCenter
	JustifyEnd
	JustifySpaceBetween
	JustifySpaceAround
	JustifySpaceEvenly
)

// Align positions children on the cross axis.
type Align uint8

const (
	// AlignAuto means "use the default": Stretch for AlignItems, the
	// parent's AlignItems for AlignSelf.
	AlignAuto Align = iota
	AlignStart
	AlignCenter
	AlignEnd
	AlignStretch
)

// Position chooses normal flow (Relative) or placement by insets (Absolute).
type Position uint8

const (
	// Relative nodes are laid out in the flow, then shifted by their
	// Top/Left (or -Bottom/-Right) insets.
	Relative Position = iota
	// Absolute nodes take no space in the flow; their insets place them
	// inside the parent's border.
	Absolute
)

// Overflow controls whether children may paint outside their parent.
type Overflow uint8

const (
	Visible Overflow = iota
	Hidden
)

// Style is a node's layout options. Port of the layout part of upstream
// RenderableOptions (Renderable.ts). The zero value matches upstream
// defaults: column, start, stretch, grow 0, shrink per FlexShrink docs.
type Style struct {
	Direction  FlexDirection
	Justify    Justify
	AlignItems Align
	AlignSelf  Align

	Width, Height       Dim
	MinWidth, MinHeight Dim
	MaxWidth, MaxHeight Dim // Auto = no maximum

	FlexGrow float64
	// FlexShrink defaults (nil) to 1, or 0 when Width or Height is a fixed
	// number of cells — the rule upstream applies. Use Float(…) to set it.
	FlexShrink *float64
	FlexBasis  Dim

	Gap     int // space between children on the main axis
	Padding Edges
	Margin  Edges

	Position                 Position
	Top, Right, Bottom, Left Dim // insets (see Position)

	Overflow Overflow
}

// Float returns a pointer to v, for optional fields like Style.FlexShrink.
func Float(v float64) *float64 { return &v }

func (s *Style) shrink() float32 {
	if s.FlexShrink != nil {
		return float32(*s.FlexShrink)
	}
	if s.Width.kind == dimCells || s.Height.kind == dimCells {
		return 0
	}
	return 1
}

// MeasureMode tells a measure function how to treat the size it is given.
type MeasureMode uint8

const (
	MeasureUndefined MeasureMode = iota // no limit: report the natural size
	MeasureExactly                      // the size is fixed: report it
	MeasureAtMost                       // the size is a maximum
)

// MeasureFunc reports the content size of a leaf node (text, for example)
// for the given constraints. Port of Yoga's measure function.
type MeasureFunc func(width float64, wm MeasureMode, height float64, hm MeasureMode) (w, h float64)

var undefined = float32(math.NaN())

func isNaN(f float32) bool   { return f != f }
func defined(f float32) bool { return f == f }

// ComputeLayout sizes root to width×height at (0, 0) and positions every
// visible node under it. Results are read with Node.Rect.
func ComputeLayout(root *Node, width, height int) {
	w, h := float32(width), float32(height)
	root.lay.pos = [4]float32{}
	root.calculate(w, h, stretchFit, stretchFit, w, h, true)
	root.roundToGrid(0, 0)
	prepareScrollTree(root)
}
