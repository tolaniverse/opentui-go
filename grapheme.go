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

func displayWidth(s string) int {
	if asciiOnly(s) {
		return len(s)
	}
	return uniseg.StringWidth(s)
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
		width := g.Width()
		if width <= 0 {
			width = 1 // control characters paint as a space
		}
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
