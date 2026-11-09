//go:build windows

package ptycheck

import (
	"os"
	"strings"
	"testing"
)

// Windows smoke tests run examples in a ConPTY pseudo console. ConPTY
// re-encodes output and handles terminal modes itself, so these check what
// the user sees (text, layout, editing, resize, exit), not the program's
// exact escape sequences.

func winStart(t *testing.T, name string, w, h int) *session {
	return startEnv(t, buildExample(t, name), w, h, os.Environ())
}

func TestWindowsHello(t *testing.T) {
	s := winStart(t, "hello", 80, 24)
	x, y := s.waitText("Hello, OpenTUI-Go!")
	if x < 20 || y < 8 {
		t.Errorf("greeting at %d,%d; want it centred on an 80x24 screen", x, y)
	}
	s.waitText("╭─")
	s.exitCleanly()
}

func TestWindowsInput(t *testing.T) {
	s := winStart(t, "input", 90, 24)
	s.waitText("Name — Enter moves to email")
	s.send("Ada")
	// ConPTY may show a cursor cell between characters in the focused
	// editor (e.g. "A da"). The submitted-value assertion below checks
	// that the program actually received the whole name.
	s.send("\t")
	s.waitText("Email — Enter submits")
	s.send("\x1b[Z") // Shift+Tab
	s.waitText("Name — Enter moves to email")
	s.send("\r")
	s.waitText("Name saved — enter your email")
	s.send("\x1b[200~ada@\nexample.com\x1b[201~") // bracketed paste
	s.waitText("ada@example.com")
	s.send("\r")
	s.waitText("Submitted: Ada <ada@example.com>")
	s.exitCleanly()
}

func TestWindowsTextareaWrapAndResize(t *testing.T) {
	s := winStart(t, "textarea", 50, 14)
	s.waitText("Ln 1, Col 1")
	text := strings.Repeat("abcdefghij", 6) + " and some words past the edge"
	s.send(text)
	s.waitText("Col 90 · 1 line(s)")
	s.waitFor("start and end both visible (wrapped)", func() bool {
		_, sy, ok1 := s.screen.Find("abcdefghij")
		_, ey, ok2 := s.screen.Find("edge")
		return ok1 && ok2 && ey > sy
	})
	s.resize(36, 14)
	s.waitFor("re-wrapped after resize", func() bool {
		for _, l := range s.screen.Lines() {
			if len([]rune(l)) > 36 {
				return false
			}
		}
		_, _, ok := s.screen.Find("edge")
		return ok
	})
	s.exitCleanly()
}
