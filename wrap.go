package opentui

import (
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// WrapMode controls soft wrapping of Text and Textarea lines. Upstream
// offers the same three modes ("none", "char", "word") and defaults to
// "word"; here the zero value is WrapNone so existing layouts do not
// change.
type WrapMode uint8

const (
	WrapNone WrapMode = iota // one row per line; overflow is clipped or scrolled
	WrapChar                 // break between any two grapheme clusters
	WrapWord                 // break at Unicode line-break opportunities
)

func (m WrapMode) String() string {
	switch m {
	case WrapChar:
		return "char"
	case WrapWord:
		return "word"
	}
	return "none"
}

// wrapRow is one visual row of a logical line: runes [start, end) and
// their width in cells, not counting trailing spaces (which hang past the
// right edge and are not painted).
type wrapRow struct {
	start, end int
	width      int
}

type wrapCluster struct {
	start, end int // rune offsets
	width      int
	space      bool
	breakAfter bool // a line-break opportunity follows this cluster
}

// wrapLine splits line into visual rows no wider than width cells. Rows
// always tile the line, never split a grapheme cluster, and each holds at
// least one cluster. A word longer than width falls back to char breaks.
// Width <= 0 or WrapNone returns one row.
func wrapLine(line []rune, width int, mode WrapMode) []wrapRow {
	if mode == WrapNone || width <= 0 || len(line) == 0 {
		return []wrapRow{{0, len(line), displayWidth(string(line))}}
	}
	if asciiOnlyRunes(line) && len(line) <= width {
		return []wrapRow{{0, len(line), len(line)}}
	}

	clusters := make([]wrapCluster, 0, len(line))
	str, state, pos := string(line), -1, 0
	for len(str) > 0 {
		var c string
		var boundaries int
		c, str, boundaries, state = uniseg.StepString(str, state)
		n := utf8.RuneCountInString(c)
		r, _ := utf8.DecodeRuneInString(c)
		clusters = append(clusters, wrapCluster{
			start: pos, end: pos + n,
			width:      clusterWidth(c, boundaries>>uniseg.ShiftWidth),
			space:      mode == WrapWord && unicode.IsSpace(r),
			breakAfter: boundaries&uniseg.MaskLine != uniseg.LineDontBreak,
		})
		pos += n
	}

	var rows []wrapRow
	rowStart := 0                  // first cluster of the current row
	used, trailing := 0, 0         // cells in the row; how many of them are trailing spaces
	lastBreak := -1                // cluster in this row after which word-breaking is allowed
	emit := func(endCluster int) { // row = clusters [rowStart, endCluster)
		rows = append(rows, wrapRow{clusters[rowStart].start, clusters[endCluster-1].end, used - trailing})
		rowStart, used, trailing, lastBreak = endCluster, 0, 0, -1
	}
	for i, c := range clusters {
		if c.space { // spaces hang: they never cause a break
			used += c.width
			trailing += c.width
			if c.breakAfter {
				lastBreak = i
			}
			continue
		}
		if used+c.width > width && i > rowStart {
			if mode == WrapWord && lastBreak >= rowStart {
				// Emit clusters up to the break; the rest of this row
				// (lastBreak+1 .. i-1) carries over to the next one.
				used, trailing = spanWidth(clusters[rowStart : lastBreak+1])
				emit(lastBreak + 1)
				used, trailing = spanWidth(clusters[rowStart:i])
			}
			if used+c.width > width && i > rowStart { // still too long: char break
				emit(i)
			}
		}
		used += c.width
		trailing = 0
		if c.breakAfter {
			lastBreak = i
		}
	}
	emit(len(clusters))
	return rows
}

// spanWidth returns the cells of clusters and how many of them are trailing
// (hanging) spaces.
func spanWidth(clusters []wrapCluster) (used, trailing int) {
	for _, c := range clusters {
		used += c.width
		if c.space {
			trailing += c.width
		} else {
			trailing = 0
		}
	}
	return used, trailing
}
