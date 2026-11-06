//go:build unix

package ptycheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// session is one example program running in a pseudo-terminal.
type session struct {
	t      *testing.T
	cmd    *exec.Cmd
	tty    *os.File
	screen *Screen
	done   chan error
}

// buildExample compiles ./examples/<name> from the repository root.
func buildExample(t *testing.T, name string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", bin, "./examples/"+name)
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

func start(t *testing.T, bin string, w, h int) *session {
	t.Helper()
	cmd := exec.Command(bin)
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		t.Fatalf("start pty: %v", err)
	}
	s := &session{t: t, cmd: cmd, tty: tty, screen: NewScreen(w, h), done: make(chan error, 1)}
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := tty.Read(buf)
			if n > 0 {
				s.screen.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { s.done <- cmd.Wait() }()
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
		}
		tty.Close()
	})
	return s
}

func (s *session) send(data string) {
	s.t.Helper()
	if _, err := s.tty.Write([]byte(data)); err != nil {
		s.t.Fatalf("write: %v", err)
	}
}

// waitFor polls the emulated screen until cond holds or time runs out.
func (s *session) waitFor(what string, cond func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("timed out waiting for %s; screen:\n%s", what, strings.Join(s.screen.Lines(), "\n"))
}

func (s *session) waitText(text string) (x, y int) {
	s.t.Helper()
	var ok bool
	s.waitFor(fmt.Sprintf("%q", text), func() bool { x, y, ok = s.screen.Find(text); return ok })
	return x, y
}

// sgrMouse encodes an SGR mouse report at zero-based cell x, y.
func sgrMouse(code, x, y int, press bool) string {
	final := 'm'
	if press {
		final = 'M'
	}
	return fmt.Sprintf("\x1b[<%d;%d;%d%c", code, x+1, y+1, final)
}

func TestHelloEndToEnd(t *testing.T) {
	s := start(t, buildExample(t, "hello"), 90, 24)

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
	s.waitText("Selected: Line 0")
	highlight := [3]uint8{137, 180, 250}
	for i := 0; i < 6; i++ {
		if c := s.screen.Cell(x+i, y); !c.BGSet || c.BG != highlight {
			t.Fatalf("cell %d of selection not highlighted: %+v", i, c)
		}
	}
	if c := s.screen.Cell(x+6, y); c.BG == highlight {
		t.Fatalf("cell past selection highlighted: %+v", c)
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

	// Ctrl+C exits cleanly and restores every terminal mode.
	s.send("\x03")
	select {
	case err := <-s.done:
		if err != nil {
			t.Fatalf("exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not exit on Ctrl+C")
	}
	time.Sleep(50 * time.Millisecond) // let the reader drain final output
	for _, mode := range []int{1003, 1006, 2004, 1049} {
		if s.screen.Mode(mode) {
			t.Errorf("mode %d left enabled after exit", mode)
		}
	}
}
