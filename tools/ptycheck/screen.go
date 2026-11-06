// Package ptycheck holds end-to-end checks that run examples in a pty.
package ptycheck

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Cell is one emulated terminal cell.
type Cell struct {
	Char rune
	BG   [3]uint8
	// BGSet is false when the terminal default background is in effect.
	BGSet bool
}

// Screen is a minimal terminal emulator. It understands exactly the output
// the opentui-go renderer produces: CUP cursor moves, SGR colors, private
// mode set/reset, erase display, OSC strings, and UTF-8 text. Checking the
// emulated screen, rather than searching raw output bytes, is necessary
// because diff rendering never re-sends cells that did not change.
type Screen struct {
	mu      sync.Mutex
	w, h    int
	cells   [][]Cell
	x, y    int
	bg      [3]uint8
	bgSet   bool
	modes   map[int]bool
	pending []byte
}

// NewScreen returns a blank w×h screen.
func NewScreen(w, h int) *Screen {
	s := &Screen{modes: map[int]bool{}}
	s.resize(w, h)
	return s
}

// Resize changes the screen size, keeping existing cells.
func (s *Screen) Resize(w, h int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resize(w, h)
}

func (s *Screen) resize(w, h int) {
	cells := make([][]Cell, h)
	for y := range cells {
		cells[y] = make([]Cell, w)
		for x := range cells[y] {
			cells[y][x].Char = ' '
			if y < s.h && x < s.w {
				cells[y][x] = s.cells[y][x]
			}
		}
	}
	s.w, s.h, s.cells = w, h, cells
}

// Mode reports whether DEC private mode n is currently set.
func (s *Screen) Mode(n int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modes[n]
}

// Lines returns every row as a string, with trailing spaces trimmed.
func (s *Screen) Lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, s.h)
	for y, row := range s.cells {
		var b strings.Builder
		for _, c := range row {
			b.WriteRune(c.Char)
		}
		out[y] = strings.TrimRight(b.String(), " ")
	}
	return out
}

// Cell returns the cell at x, y (zero-based).
func (s *Screen) Cell(x, y int) Cell {
	s.mu.Lock()
	defer s.mu.Unlock()
	if y < 0 || y >= s.h || x < 0 || x >= s.w {
		return Cell{}
	}
	return s.cells[y][x]
}

// Find returns the row and rune column of the first occurrence of text.
func (s *Screen) Find(text string) (x, y int, ok bool) {
	for row, line := range s.Lines() {
		if i := strings.Index(line, text); i >= 0 {
			return utf8.RuneCountInString(line[:i]), row, true
		}
	}
	return 0, 0, false
}

// Write feeds terminal output. Sequences split across writes are retained.
func (s *Screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := append(s.pending, p...)
	s.pending = nil
	i := 0
	for i < len(data) {
		n, complete := s.step(data[i:])
		if !complete {
			s.pending = append([]byte(nil), data[i:]...)
			break
		}
		i += n
	}
	return len(p), nil
}

// step consumes one control sequence or rune.
func (s *Screen) step(b []byte) (int, bool) {
	if b[0] != 0x1b {
		if !utf8.FullRune(b) {
			return 0, false
		}
		r, n := utf8.DecodeRune(b)
		s.put(r)
		return n, true
	}
	if len(b) < 2 {
		return 0, false
	}
	switch b[1] {
	case '[':
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				s.csi(string(b[2:i]), b[i])
				return i + 1, true
			}
		}
		return 0, false
	case ']':
		for i := 2; i < len(b); i++ {
			if b[i] == 0x07 {
				return i + 1, true
			}
			if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
				return i + 2, true
			}
		}
		return 0, false
	}
	return 2, true // two-byte escape: ignored
}

func (s *Screen) put(r rune) {
	switch r {
	case '\r':
		s.x = 0
		return
	case '\n':
		s.y++
		return
	}
	if r < 0x20 {
		return
	}
	if s.y >= 0 && s.y < s.h && s.x >= 0 && s.x < s.w {
		s.cells[s.y][s.x] = Cell{Char: r, BG: s.bg, BGSet: s.bgSet}
	}
	s.x++
}

func (s *Screen) csi(params string, final byte) {
	if strings.HasPrefix(params, "?") {
		if final == 'h' || final == 'l' {
			for _, p := range strings.Split(params[1:], ";") {
				if n, err := strconv.Atoi(p); err == nil {
					s.modes[n] = final == 'h'
				}
			}
		}
		return
	}
	args := strings.Split(params, ";")
	num := func(i, def int) int {
		if i < len(args) {
			if n, err := strconv.Atoi(args[i]); err == nil {
				return n
			}
		}
		return def
	}
	switch final {
	case 'H':
		s.y, s.x = num(0, 1)-1, num(1, 1)-1
	case 'J':
		if num(0, 0) == 2 {
			for y := range s.cells {
				for x := range s.cells[y] {
					s.cells[y][x] = Cell{Char: ' '}
				}
			}
		}
	case 'm':
		s.sgr(args)
	}
}

func (s *Screen) sgr(args []string) {
	for i := 0; i < len(args); i++ {
		n, _ := strconv.Atoi(args[i])
		switch {
		case n == 0:
			s.bgSet = false
		case n == 49:
			s.bgSet = false
		case n == 48 && i+4 < len(args) && args[i+1] == "2":
			r, _ := strconv.Atoi(args[i+2])
			g, _ := strconv.Atoi(args[i+3])
			b, _ := strconv.Atoi(args[i+4])
			s.bg, s.bgSet = [3]uint8{uint8(r), uint8(g), uint8(b)}, true
			i += 4
		case (n == 38 || n == 48) && i+1 < len(args) && args[i+1] == "2":
			i += 4
		case (n == 38 || n == 48) && i+1 < len(args) && args[i+1] == "5":
			i += 2
		}
	}
}
