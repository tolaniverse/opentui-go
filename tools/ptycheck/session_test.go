package ptycheck

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// terminal is a pseudo-terminal running one program: a Unix PTY
// (pty_unix_test.go) or a Windows pseudo console (conpty_windows_test.go).
// Reads return what the program draws; writes are typed input.
type terminal interface {
	io.ReadWriter
	Resize(w, h int) error
	Kill()
	Wait() error // waits for the program to exit
	Close() error
}

// session is one example program running in a pseudo-terminal.
type session struct {
	t       *testing.T
	term    terminal
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
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./examples/"+name)
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

// start runs bin with a fixed truecolor terminal environment, so color
// assertions do not depend on the terminal or CI running the tests.
func start(t *testing.T, bin string, w, h int) *session {
	return startEnv(t, bin, w, h, cleanEnv("TERM=xterm-256color", "COLORTERM=truecolor"))
}

// cleanEnv is the test process environment without terminal-detection
// variables, so each case sets exactly what it tests.
func cleanEnv(extra ...string) []string {
	skip := map[string]bool{"TERM": true, "COLORTERM": true, "NO_COLOR": true, "WT_SESSION": true,
		"TERM_PROGRAM": true, "VTE_VERSION": true, "TMUX": true, "STY": true, "ZELLIJ": true,
		"FORCE_HYPERLINK": true, "WSL_DISTRO_NAME": true, "WSL_INTEROP": true}
	var out []string
	for _, e := range os.Environ() {
		if name, _, _ := strings.Cut(e, "="); !skip[name] {
			out = append(out, e)
		}
	}
	return append(out, extra...)
}

func startEnv(t *testing.T, bin string, w, h int, env []string) *session {
	t.Helper()
	term, err := startTerminal(bin, env, w, h)
	if err != nil {
		t.Fatalf("start %s: %v", filepath.Base(bin), err)
	}
	s := &session{t: t, term: term, screen: NewScreen(w, h), exited: make(chan struct{})}
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := term.Read(buf)
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
	go func() { s.waitErr = term.Wait(); close(s.exited) }()
	t.Cleanup(func() {
		// Only the Wait goroutine may observe the exit, so always kill
		// (harmless after exit) and then wait for that goroutine.
		term.Kill()
		select {
		case <-s.exited:
		case <-time.After(5 * time.Second):
		}
		term.Close()
	})
	return s
}

// resize changes the terminal size and the emulated screen together.
func (s *session) resize(w, h int) {
	s.t.Helper()
	s.screen.Resize(w, h)
	if err := s.term.Resize(w, h); err != nil {
		s.t.Fatal(err)
	}
}

func (s *session) send(data string) {
	s.t.Helper()
	if _, err := s.term.Write([]byte(data)); err != nil {
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
	s.rawMu.Lock()
	raw := s.raw.String()
	s.rawMu.Unlock()
	if len(raw) > 4096 {
		raw = raw[:4096] + "... (truncated)"
	}
	exit := "running"
	select {
	case <-s.exited:
		exit = fmt.Sprint(s.waitErr)
	default:
	}
	s.t.Fatalf("timed out waiting for %s (child: %s); screen:\n%s\nraw output: %q", what, exit, strings.Join(s.screen.Lines(), "\n"), raw)
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
	// ConPTY re-encodes output and owns private modes, so a Windows run
	// cannot prove the process reset them. A Unix PTY shows the raw sequences.
	if runtime.GOOS == "windows" {
		return
	}
	for _, mode := range []int{1003, 1006, 2004, 1049} {
		if s.screen.Mode(mode) {
			s.t.Errorf("mode %d left enabled after exit", mode)
		}
	}
}
