package opentui

import (
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

func asciiOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// clusterWidth is the number of cells a grapheme cluster occupies, given
// uniseg's width for it. Every measuring, wrapping, editing and painting
// path uses this one rule: control characters take one cell because they
// are painted as a space; other clusters use uniseg's width, which is 0
// for a lone combining mark or a zero-width space (not painted).
func clusterWidth(cluster string, width int) int {
	if r, _ := utf8.DecodeRuneInString(cluster); r < 0x20 || r == 0x7f {
		return 1
	}
	return width
}

func displayWidth(s string) int {
	if asciiOnly(s) {
		return len(s)
	}
	width := 0
	for g := uniseg.NewGraphemes(s); g.Next(); {
		width += clusterWidth(g.Str(), g.Width())
	}
	return width
}

type textCluster struct {
	text       string
	start, end int // terminal cell columns, not byte or rune offsets
}

func clusters(line string) []textCluster {
	var result []textCluster
	column := 0
	g := uniseg.NewGraphemes(line)
	for g.Next() {
		width := clusterWidth(g.Str(), g.Width())
		result = append(result, textCluster{g.Str(), column, column + width})
		column += width
	}
	return result
}

// selectedColumns expands the selection to whole grapheme clusters.
func selectedColumns(line string, start, end int) (text string, from, to int) {
	from = -1
	for _, c := range clusters(line) {
		if c.start >= end || c.end <= start {
			continue
		}
		if from < 0 {
			from = c.start
		}
		to = c.end
		text += c.text
	}
	if from < 0 {
		return "", 0, 0
	}
	return text, from, to
}

func wordColumns(line string, cell int) (from, to int) {
	cs := clusters(line)
	if len(cs) == 0 {
		return 0, 0
	}
	i := len(cs) - 1
	for j, c := range cs {
		if cell < c.end {
			i = j
			break
		}
	}
	isWord := func(c textCluster) bool {
		r, _ := utf8.DecodeRuneInString(c.text)
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
	}
	class := isWord(cs[i])
	left, right := i, i+1
	for left > 0 && isWord(cs[left-1]) == class {
		left--
	}
	for right < len(cs) && isWord(cs[right]) == class {
		right++
	}
	return cs[left].start, cs[right-1].end
}
