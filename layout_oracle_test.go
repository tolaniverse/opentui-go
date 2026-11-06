package opentui

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

// The cases in testdata/yoga_cases.json were laid out by real Yoga 3.2.1
// (see tools/yoga-oracle). Every rect our engine computes must match.

type oracleFile struct {
	Yoga  string       `json:"yoga"`
	Cases []oracleCase `json:"cases"`
}

type oracleCase struct {
	Name         string     `json:"name"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	OpenTUIPatch string     `json:"openTUIPatch"`
	Root         oracleNode `json:"root"`
	Want         [][4]int   `json:"want"`
}

type oracleNode struct {
	S      map[string]any `json:"s"`
	Border []int          `json:"border"`
	Text   []int          `json:"text"`
	Hidden bool           `json:"hidden"`
	C      []oracleNode   `json:"c"`
}

func oracleDim(v any) Dim {
	switch x := v.(type) {
	case float64:
		return Dim{dimCells, float32(x)}
	case string:
		var p float64
		fmt.Sscanf(strings.TrimSuffix(x, "%"), "%g", &p)
		return Pct(p)
	}
	return Auto
}

func oracleEdges(v any) Edges {
	a, _ := v.([]any)
	if len(a) != 4 {
		return Edges{}
	}
	return Edges{int(a[0].(float64)), int(a[1].(float64)), int(a[2].(float64)), int(a[3].(float64))}
}

func buildOracle(spec oracleNode) *Node {
	s := spec.S
	st := Style{
		Width: oracleDim(s["w"]), Height: oracleDim(s["h"]),
		MinWidth: oracleDim(s["minW"]), MinHeight: oracleDim(s["minH"]),
		MaxWidth: oracleDim(s["maxW"]), MaxHeight: oracleDim(s["maxH"]),
		FlexBasis: oracleDim(s["basis"]),
		Padding:   oracleEdges(s["pad"]), Margin: oracleEdges(s["margin"]),
		Top: oracleDim(s["top"]), Right: oracleDim(s["right"]),
		Bottom: oracleDim(s["bottom"]), Left: oracleDim(s["left"]),
	}
	st.Direction = map[any]FlexDirection{"row": Row, "column-reverse": ColumnReverse, "row-reverse": RowReverse}[s["dir"]]
	st.Justify = map[any]Justify{"center": JustifyCenter, "end": JustifyEnd, "between": JustifySpaceBetween, "around": JustifySpaceAround, "evenly": JustifySpaceEvenly}[s["justify"]]
	aligns := map[any]Align{"start": AlignStart, "center": AlignCenter, "end": AlignEnd, "stretch": AlignStretch}
	st.AlignItems, st.AlignSelf = aligns[s["alignItems"]], aligns[s["alignSelf"]]
	if g, ok := s["grow"].(float64); ok {
		st.FlexGrow = g
	}
	if v, ok := s["shrink"].(float64); ok {
		st.FlexShrink = Float(v)
	}
	if g, ok := s["gap"].(float64); ok {
		st.Gap = int(g)
	}
	if s["pos"] == "abs" {
		st.Position = Absolute
	}

	n := NewNode(st)
	n.Hidden = spec.Hidden
	if len(spec.Border) == 4 {
		n.border = Edges{spec.Border[0], spec.Border[1], spec.Border[2], spec.Border[3]}
	}
	if len(spec.Text) == 2 {
		tw, th := float64(spec.Text[0]), float64(spec.Text[1])
		n.measure = func(w float64, wm MeasureMode, h float64, hm MeasureMode) (float64, float64) {
			return fit(tw, w, wm), fit(th, h, hm) // same as the JS side
		}
	}
	for _, c := range spec.C {
		n.Add(buildOracle(c))
	}
	return n
}

func collectRects(spec oracleNode, n *Node, out *[][4]int) {
	if spec.Hidden {
		return
	}
	r := n.Rect()
	*out = append(*out, [4]int{r.X, r.Y, r.W, r.H})
	for i, c := range spec.C {
		collectRects(c, n.children[i], out)
	}
}

func TestLayoutMatchesYoga(t *testing.T) {
	data, err := os.ReadFile("testdata/yoga_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var f oracleFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	failed := 0
	var failures []string
	only := os.Getenv("ORACLE_CASE") // debug: dump one case
	for _, tc := range f.Cases {
		if only != "" && tc.Name != only {
			continue
		}
		root := buildOracle(tc.Root)
		ComputeLayout(root, tc.Width, tc.Height)
		var got [][4]int
		collectRects(tc.Root, root, &got)
		if only != "" {
			spec, _ := json.MarshalIndent(tc.Root, "", "  ")
			t.Logf("%s %dx%d\n%s\ngot  %v\nyoga %v", tc.Name, tc.Width, tc.Height, spec, got, tc.Want)
		}
		if tc.OpenTUIPatch == "flex-shrink-residue" {
			// Unpatched Yoga divides by a near-zero shrink total and emits
			// dimensions in the hundreds of millions. OpenTUI's patch treats
			// that residue as zero. Verify the patched invariant instead of
			// copying npm Yoga's corrupted rectangles.
			for i, r := range got {
				for _, v := range r {
					if v < -1_000_000 || v > 1_000_000 {
						t.Fatalf("%s: patched node #%d still overflowed: %v", tc.Name, i, r)
					}
				}
			}
			continue
		}
		for i := range tc.Want {
			if i >= len(got) || got[i] != tc.Want[i] {
				failed++
				failures = append(failures, fmt.Sprintf("%s(%d nodes)", tc.Name, len(tc.Want)))
				if failed <= 5 {
					g := [4]int{}
					if i < len(got) {
						g = got[i]
					}
					t.Errorf("%s: node #%d got %v, yoga %v", tc.Name, i, g, tc.Want[i])
				}
				break
			}
		}
	}
	t.Logf("Yoga %s: %d/%d cases match", f.Yoga, len(f.Cases)-failed, len(f.Cases))
	if failed > 0 {
		slices.SortFunc(failures, func(a, b string) int { return len(a) - len(b) })
		t.Logf("failing: %v", failures[:min(len(failures), 12)])
		t.Errorf("%d of %d cases differ from Yoga", failed, len(f.Cases))
	}
}
