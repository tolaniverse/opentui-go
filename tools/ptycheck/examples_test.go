//go:build unix

package ptycheck

import (
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
