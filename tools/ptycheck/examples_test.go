//go:build unix

package ptycheck

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/creack/pty"
)

func TestHello(t *testing.T) {
	s := start(t, buildExample(t, "hello"), 80, 24)
	x, y := s.waitText("Hello, OpenTUI-Go!")
	if x < 20 || y < 8 {
		t.Errorf("greeting at %d,%d; want it centred on an 80x24 screen", x, y)
	}
	s.waitText("╭─────") // rounded border drawn
	s.exitCleanly()
}

func TestUnicode(t *testing.T) {
	s := start(t, buildExample(t, "unicode"), 40, 12)
	s.waitFor("Unicode terminal-cell alignment", func() bool {
		return s.screen.Cell(3, 0).Char == 'B' &&
			s.screen.Cell(1, 1).Char == 'X' &&
			s.screen.Cell(2, 2).Char == '!' &&
			s.screen.Cell(0, 2).Grapheme == "👩🏽‍💻"
	})
	s.exitCleanly()
}

var clockRE = regexp.MustCompile(`\d\d:\d\d:\d\d`)

func TestLayout(t *testing.T) {
	s := start(t, buildExample(t, "layout"), 90, 24)
	s.waitText("Components")
	s.waitText("Live layout")

	// The clock is updated from another goroutine through Renderer.Update.
	clock := func() string { return clockRE.FindString(strings.Join(s.screen.Lines(), "\n")) }
	s.waitFor("clock", func() bool { return clock() != "" })
	first := clock()
	s.waitFor("clock to tick", func() bool { return clock() != "" && clock() != first })

	// Both panels share one row; after a resize the row spans the new width.
	panelsSpan := func(width int) func() bool {
		return func() bool {
			for _, line := range s.screen.Lines() {
				if strings.HasPrefix(line, "┌─ Components") && strings.Contains(line, "Live layout") &&
					len([]rune(line)) == width {
					return true
				}
			}
			return false
		}
	}
	s.waitFor("panels span 90 columns", panelsSpan(90))
	s.screen.Resize(60, 20)
	if err := pty.Setsize(s.tty, &pty.Winsize{Cols: 60, Rows: 20}); err != nil {
		t.Fatal(err)
	}
	s.waitFor("panels reflow to 60 columns", panelsSpan(60))
	s.exitCleanly()
}

func TestInput(t *testing.T) {
	s := start(t, buildExample(t, "input"), 90, 24)
	// The name field is focused at startup, so its focus message shows first.
	s.waitText("Name — Enter moves to email")

	s.send("Ada")
	s.waitText("│ Ada")
	s.send("\t")
	s.waitText("Email — Enter submits")
	s.send("\x1b[Z") // Shift+Tab
	s.waitText("Name — Enter moves to email")
	s.send("\r") // submitting the name moves focus to the email field
	s.waitText("Name saved — enter your email")

	// Bracketed paste: the newline is stripped by the single-line input.
	s.send("\x1b[200~ada@\nexample.com\x1b[201~")
	s.waitText("ada@example.com")
	s.send("\r")
	s.waitText("Submitted: Ada <ada@example.com>")

	s.send("\x1b") // lone Escape, resolved after the decoder's timeout
	s.waitText("Focus cleared — press Tab to resume")
	s.exitCleanly()
}

func TestInputUnicodeEditing(t *testing.T) {
	s := start(t, buildExample(t, "input"), 90, 24)
	s.waitText("Name — Enter moves to email")
	s.send("\x1b[200~A👩🏽‍💻B\x1b[201~")
	s.send("\x1b[D") // before B
	s.send("\x7f")   // remove the whole emoji cluster, not its final rune
	s.send("\r")
	s.waitText("Name saved — enter your email")
	s.send("ada@example.com\r")
	s.waitText("Submitted: AB <ada@example.com>")
	s.exitCleanly()
}

func TestMouse(t *testing.T) {
	s := start(t, buildExample(t, "mouse"), 90, 24)
	s.waitText("Move, click, or drag the card")

	// The interior of a zone: its title sits on the top border, three cells
	// right of the box's left edge, so the title column is inside the box.
	zone := func(name string) (x, y int) {
		tx, ty := s.waitText("Zone " + name)
		return tx, ty + 3
	}
	ax, _ := zone("A")
	bx, by := zone("B")
	cx, cy := zone("C")

	s.send(sgrMouse(35, bx, by, true)) // motion with no button held
	s.waitText("Hovering zone B")
	s.send(sgrMouse(0, bx, by, true))
	s.send(sgrMouse(0, bx, by, false))
	s.waitText("Clicked zone B (1)")

	// Drag the card from zone A and drop it on zone C.
	kx, ky := s.waitText("card")
	if kx < ax || kx >= bx {
		t.Fatalf("card starts at column %d, outside zone A (%d..%d)", kx, ax, bx)
	}
	s.send(sgrMouse(0, kx, ky, true))
	s.waitText("Picked up the card")
	s.send(sgrMouse(32, (kx+cx)/2, cy, true))
	s.send(sgrMouse(32, cx, cy, true))
	s.send(sgrMouse(0, cx, cy, false))
	s.waitText("Dropped the card on zone C")
	s.waitFor("card drawn in zone C", func() bool {
		x, _, ok := s.screen.Find("card")
		return ok && x >= cx-3
	})

	// The press on the card stopped propagation, so zone A counted nothing.
	all := strings.Join(s.screen.Lines(), "\n")
	if strings.Count(all, "clicks: 0") != 2 || strings.Count(all, "clicks: 1") != 1 {
		t.Fatalf("click counts wrong:\n%s", all)
	}
	s.exitCleanly()
}

func TestTextarea(t *testing.T) {
	s := start(t, buildExample(t, "textarea"), 80, 16)
	s.waitText("Ln 1, Col 1 · 1 line(s)")

	s.send("first")
	s.send("\r")
	s.send("second")
	s.waitText("Ln 2, Col 7 · 2 line(s)")
	_, top, ok := s.screen.Find("first")
	if _, below, ok2 := s.screen.Find("second"); !ok || !ok2 || below != top+1 {
		t.Fatalf("lines not stacked: first ok=%v second ok=%v", ok, ok2)
	}

	// Up keeps the column (clamped to "first"), Backspace at a line start
	// joins lines.
	s.send("\x1b[A")
	s.waitText("Ln 1, Col 6")
	s.send("\x1b[B\x01\x7f") // down, Ctrl+A, Backspace
	s.waitText("firstsecond")
	s.waitText("Ln 1, Col 6 · 1 line(s)")

	// Multi-line bracketed paste with CRLF line endings and an emoji.
	s.send("\x05\x1b[200~\r\nA👩🏽‍💻B\r\nlast\x1b[201~")
	s.waitText("Ln 3, Col 5 · 3 line(s)")
	s.waitText("A👩🏽‍💻B")
	s.send("\x1b[A\x05\x1b[D\x7f") // up, Ctrl+E, left, Backspace: removes the emoji
	s.waitText("Ln 2, Col 2 · 3 line(s)")
	if _, _, ok := s.screen.Find("👩🏽‍💻"); ok {
		t.Fatal("emoji cluster not deleted as one unit")
	}

	// Enough lines to scroll: the cursor's line stays visible.
	s.send("\x1b[F" + strings.Repeat("\rmore", 20))
	s.waitText("Ln 23, Col 5 · 23 line(s)")
	s.waitFor("view scrolled to the last line", func() bool {
		_, _, gone := s.screen.Find("firstsecond")
		return !gone
	})

	s.send("\x1b\r") // Alt+Enter
	s.waitText("Submitted 23 line(s)")
	s.exitCleanly()
}

var articleRowsRE = regexp.MustCompile(`Rows (\d+) · Ln (\d+), Col (\d+)`)

func TestWrap(t *testing.T) {
	s := start(t, buildExample(t, "wrap"), 100, 24)
	s.waitText("END.")
	status := func() (rows, line, col int) {
		m := articleRowsRE.FindStringSubmatch(strings.Join(s.screen.Lines(), "\n"))
		if m == nil {
			return -1, -1, -1
		}
		fmt.Sscan(m[1], &rows)
		fmt.Sscan(m[2], &line)
		fmt.Sscan(m[3], &col)
		return rows, line, col
	}
	s.waitFor("status", func() bool { r, _, _ := status(); return r > 1 })
	wide, _, _ := status()

	// Narrower terminal: the article reflows onto more rows.
	s.screen.Resize(60, 24)
	if err := pty.Setsize(s.tty, &pty.Winsize{Cols: 60, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	s.waitFor("article reflows to more rows", func() bool { r, _, _ := status(); return r > wide })
	s.waitText("END.")

	// A long editor line wraps across visual rows but stays one line.
	sentence := "alpha beta gamma delta epsilon zeta eta theta iota kappa"
	s.send(sentence)
	s.waitFor("cursor at end of the sentence", func() bool {
		_, line, col := status()
		return line == 1 && col == len(sentence)+1
	})
	_, alphaY, ok1 := s.screen.Find("alpha")
	_, kappaY, ok2 := s.screen.Find("kappa")
	if !ok1 || !ok2 || kappaY <= alphaY {
		t.Fatalf("editor line did not wrap: alpha row %d (%v), kappa row %d (%v)", alphaY, ok1, kappaY, ok2)
	}
	// Up moves one visual row within the same logical line.
	s.send("\x1b[A")
	s.waitFor("up moves within the wrapped line", func() bool {
		_, line, col := status()
		return line == 1 && col > 1 && col < len(sentence)+1
	})

	// Selecting across a soft wrap in the article joins rows without "\n".
	x, y := s.waitText("Soft wrapping")
	s.send(sgrMouse(0, x, y, true))
	s.send(sgrMouse(32, x+3, y+1, true))
	s.send(sgrMouse(0, x+3, y+1, false))
	// The first row runs from x to the panel's right border.
	firstRow := []rune(s.screen.Lines()[y])[x:]
	firstRow = firstRow[:strings.IndexRune(string(firstRow), '│')]
	firstLen := len([]rune(strings.TrimRight(string(firstRow), " ")))
	selRE := regexp.MustCompile(`Sel (\d+) chars, (\d+) newlines`)
	s.waitFor("selection status", func() bool { return selRE.MatchString(strings.Join(s.screen.Lines(), "\n")) })
	m := selRE.FindStringSubmatch(strings.Join(s.screen.Lines(), "\n"))
	var chars, newlines int
	fmt.Sscan(m[1], &chars)
	fmt.Sscan(m[2], &newlines)
	if chars <= firstLen || newlines != 0 {
		t.Fatalf("selection across a soft wrap: %d chars (first row %d), %d newlines; want more chars and 0 newlines", chars, firstLen, newlines)
	}
	s.exitCleanly()
}

func TestTextareaMouse(t *testing.T) {
	s := start(t, buildExample(t, "textarea"), 80, 16)
	s.waitText("Ln 1, Col 1 · 1 line(s)")
	s.send("first\rsecond\rthird")
	s.waitText("Ln 3, Col 6 · 3 line(s)")
	fx, fy := s.waitText("first")
	sx, sy := s.waitText("second")

	// Click places the cursor.
	s.send(sgrMouse(0, sx+3, sy, true))
	s.send(sgrMouse(0, sx+3, sy, false))
	s.waitText("Ln 2, Col 4 · 3 line(s)")

	// Drag from "first" to the middle of "second" selects and copies.
	s.send(sgrMouse(0, fx, fy, true))
	s.send(sgrMouse(32, sx+2, sy, true))
	s.send(sgrMouse(32, sx+3, sy, true))
	s.send(sgrMouse(0, sx+3, sy, false))
	s.waitText("Copied 9 chars")
	osc52 := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("first\nsec")) + "\x1b\\"
	s.waitFor("OSC 52 copy of the editor selection", func() bool { return s.rawContains(osc52) })

	// Typing replaces the selection.
	s.send("Z")
	s.waitText("Zond")
	s.waitText("Ln 1, Col 2 · 2 line(s)")

	// Fill past the viewport, then wheel up: the view moves, the cursor
	// does not; the next key scrolls back to the cursor.
	s.send("\x1b[F" + strings.Repeat("\rmore", 20))
	s.waitText("Ln 22, Col 5 · 22 line(s)")
	s.waitFor("first line scrolled out", func() bool { _, _, ok := s.screen.Find("Zond"); return !ok })
	for i := 0; i < 10; i++ {
		s.send(sgrMouse(64, sx, sy, true))
	}
	s.waitText("Zond")
	s.waitText("Ln 22, Col 5 · 22 line(s)")
	s.send("!")
	s.waitFor("view follows the cursor again", func() bool { _, _, ok := s.screen.Find("Zond"); return !ok })
	s.waitText("Ln 22, Col 6")
	s.exitCleanly()
}

func TestColorModes(t *testing.T) {
	bin := buildExample(t, "color")
	base := os.Environ()
	strip := func(keys ...string) []string {
		var out []string
		skip := map[string]bool{}
		for _, k := range keys {
			skip[k] = true
		}
		for _, e := range base {
			name, _, _ := strings.Cut(e, "=")
			if !skip[name] {
				out = append(out, e)
			}
		}
		return out
	}
	for _, tc := range []struct {
		env             []string
		text, has, lack string
	}{
		{[]string{"TERM=xterm-256color", "COLORTERM=truecolor"}, "mode truecolor", "\x1b[38;2;250;10;10m", "38;5;"},
		{[]string{"TERM=xterm-256color"}, "mode 256", "\x1b[38;5;9m", "38;2;"},
		{[]string{"TERM=linux"}, "mode 16", "\x1b[91m", "38;"},
		{[]string{"TERM=xterm-256color", "COLORTERM=truecolor", "NO_COLOR=1"}, "mode none", "\x1b[39m", "38;"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			s := startEnv(t, bin, 40, 8, append(strip("TERM", "COLORTERM", "NO_COLOR", "WT_SESSION"), tc.env...))
			s.waitText(tc.text)
			s.waitFor("color sequence", func() bool { return s.rawContains(tc.has) })
			if s.rawContains(tc.lack) {
				t.Fatalf("output contains %q, which this mode must not send", tc.lack)
			}
			s.exitCleanly()
		})
	}
}

func TestTextareaUndoAndWords(t *testing.T) {
	s := start(t, buildExample(t, "textarea"), 80, 16)
	s.waitText("Ln 1, Col 1 · 1 line(s)")
	s.send("alpha beta gamma")
	s.waitText("Ln 1, Col 17")
	s.send("\x1b[1;5D") // Ctrl+Left: start of "gamma"
	s.waitText("Ln 1, Col 12")
	s.send("\x1bb") // Alt+B: start of "beta"
	s.waitText("Ln 1, Col 7")
	s.send("\x1bd") // Alt+D: delete "beta "
	s.waitText("alpha gamma")
	s.send("\x1a") // Ctrl+Z: undo the word delete
	s.waitText("alpha beta gamma")
	s.send("\x1a\x1a") // undo " gamma", then " beta"
	s.waitFor("typing undone by word", func() bool {
		_, _, gamma := s.screen.Find("gamma")
		_, _, beta := s.screen.Find("beta")
		_, _, alpha := s.screen.Find("alpha")
		return alpha && !beta && !gamma
	})
	s.waitText("Ln 1, Col 6")
	s.send("\x19") // Ctrl+Y: redo " beta"
	s.waitText("alpha beta")
	s.waitText("Ln 1, Col 11")
	s.exitCleanly()
}

func TestInputUndo(t *testing.T) {
	s := start(t, buildExample(t, "input"), 90, 24)
	s.waitText("Name — Enter moves to email")
	s.send("Ada King")
	s.waitText("│ Ada King")
	s.send("\x17") // Ctrl+W deletes "King"
	s.waitFor("word deleted", func() bool { _, _, ok := s.screen.Find("King"); return !ok })
	s.send("\x1a") // Ctrl+Z restores it
	s.waitText("│ Ada King")
	s.send("\r")
	s.waitText("Name saved — enter your email")
	s.send("a@b.c\r")
	s.waitText("Submitted: Ada King <a@b.c>")
	s.exitCleanly()
}
