//go:build unix

package ptycheck

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/creack/pty"
)

func TestDocument(t *testing.T) {
	s := start(t, buildExample(t, "document"), 90, 24)

	x, y := s.waitText("Line 01")
	s.waitFor("mouse, SGR and paste modes", func() bool {
		return s.screen.Mode(1003) && s.screen.Mode(1006) && s.screen.Mode(2004) && s.screen.Mode(1049)
	})

	// Wheel down over the document scrolls by the default step of 3 lines.
	s.send(sgrMouse(65, x+2, y, true))
	s.waitFor("wheel scroll", func() bool {
		_, _, gone := s.screen.Find("Line 01")
		_, top, ok := s.screen.Find("Line 04")
		return !gone && ok && top == y
	})

	// Drag across the first six cells of the top document line.
	s.send(sgrMouse(0, x, y, true))
	s.send(sgrMouse(32, x+5, y, true))
	s.send(sgrMouse(0, x+5, y, false))
	s.waitText("Copied 1 line(s): Line 0")
	osc52 := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("Line 0")) + "\x1b\\"
	s.waitFor("OSC 52 copy of the selection", func() bool { return s.rawContains(osc52) })
	highlight := [3]uint8{137, 180, 250}
	for i := 0; i < 6; i++ {
		if c := s.screen.Cell(x+i, y); !c.BGSet || c.BG != highlight {
			t.Fatalf("cell %d of selection not highlighted: %+v", i, c)
		}
	}
	if c := s.screen.Cell(x+6, y); c.BG == highlight {
		t.Fatalf("cell past selection highlighted: %+v", c)
	}

	// Scrollbar: the bar is the viewport's last column, just inside the
	// right border (ScrollBox padding belongs to the scrolled content).
	row := []rune(s.screen.Lines()[y])
	barX := len(row) - 2
	thumbRow := func() int {
		for r := y; r < y+30; r++ {
			if c := s.screen.Cell(barX, r); c.BGSet && c.BG == highlight {
				return r
			}
		}
		return -1
	}
	s.waitFor("scrollbar thumb", func() bool { return thumbRow() >= 0 })
	bottom := y
	for s.screen.Cell(barX, bottom+1).BGSet && strings.HasPrefix(s.screen.Lines()[bottom+1], "│") {
		bottom++
	}
	s.send(sgrMouse(0, barX, thumbRow(), true))
	s.send(sgrMouse(32, barX, bottom, true))
	s.send(sgrMouse(0, barX, bottom, false))
	s.waitText("Line 24")
	trackTop := y - 1 // the content's top padding row sits above Line 01
	s.send(sgrMouse(0, barX, thumbRow(), true))
	s.send(sgrMouse(32, barX, trackTop, true))
	s.send(sgrMouse(0, barX, trackTop, false))
	s.waitFor("thumb dragged back to top", func() bool {
		_, top, ok := s.screen.Find("Line 01")
		return ok && top == y
	})

	// Auto-scroll: hold a selection drag below the document; the box keeps
	// scrolling until the end, and releasing copies every line.
	s.send(sgrMouse(0, x, y, true))
	s.send(sgrMouse(32, x+3, bottom+3, true))
	s.waitText("Line 24")
	s.send(sgrMouse(0, x+3, bottom+3, false))
	s.waitText("Copied 24 line(s): Line 01")
	// Every visible document row must be complete text or a blank padding
	// row: highlight and diff rendering must not leave partial rows behind.
	for r := y; r <= bottom; r++ {
		line := s.screen.Lines()[r]
		inner := strings.TrimSpace(strings.Trim(line, "│"))
		if inner != "" && !strings.HasSuffix(inner, "Select and scroll this document.") {
			t.Fatalf("row %d partially drawn: %q\n%s", r, line, strings.Join(s.screen.Lines(), "\n"))
		}
	}

	// Resize: the document box must reflow to the new width.
	s.screen.Resize(110, 30)
	if err := pty.Setsize(s.tty, &pty.Winsize{Cols: 110, Rows: 30}); err != nil {
		t.Fatalf("resize: %v", err)
	}
	s.waitFor("reflow to 110 columns", func() bool {
		for _, line := range s.screen.Lines() {
			if strings.HasPrefix(line, "┌─ Document") && len([]rune(line)) == 110 {
				return true
			}
		}
		return false
	})

	s.exitCleanly()
}
