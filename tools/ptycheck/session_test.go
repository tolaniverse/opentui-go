//go:build unix

package ptycheck

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// session is one example program running in a pseudo-terminal.
type session struct {
	t       *testing.T
	cmd     *exec.Cmd
	tty     *os.File
	screen  *Screen
	exited  chan struct{} // closed when the program exits; then waitErr is set
	waitErr error

	rawMu sync.Mutex
	raw   bytes.Buffer // everything the program wrote, for non-screen output
}

func (s *session) rawContains(sub string) bool {
	s.rawMu.Lock()
	defer s.rawMu.Unlock()
	return bytes.Contains(s.raw.Bytes(), []byte(sub))
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
	return startEnv(t, bin, w, h, nil)
}

func startEnv(t *testing.T, bin string, w, h int, env []string) *session {
	t.Helper()
	cmd := exec.Command(bin)
	if env != nil {
		cmd.Env = env
	}
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		t.Fatalf("start pty: %v", err)
	}
	s := &session{t: t, cmd: cmd, tty: tty, screen: NewScreen(w, h), exited: make(chan struct{})}
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := tty.Read(buf)
			if n > 0 {
				s.screen.Write(buf[:n])
				s.rawMu.Lock()
				s.raw.Write(buf[:n])
				s.rawMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { s.waitErr = cmd.Wait(); close(s.exited) }()
	t.Cleanup(func() {
		// Only the Wait goroutine may touch cmd.ProcessState, so always kill
		// (harmless after exit) and then wait for that goroutine to finish.
		cmd.Process.Kill()
		select {
		case <-s.exited:
		case <-time.After(5 * time.Second):
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

// exitCleanly sends Ctrl+C and checks the program exits with status 0
// and resets every terminal mode the renderer enabled.
func (s *session) exitCleanly() {
	s.t.Helper()
	s.send("\x03")
	select {
	case <-s.exited:
		if s.waitErr != nil {
			s.t.Fatalf("exit: %v", s.waitErr)
		}
	case <-time.After(5 * time.Second):
		s.t.Fatal("did not exit on Ctrl+C")
	}
	time.Sleep(50 * time.Millisecond) // let the reader drain final output
	for _, mode := range []int{1003, 1006, 2004, 1049} {
		if s.screen.Mode(mode) {
			s.t.Errorf("mode %d left enabled after exit", mode)
		}
	}
}
