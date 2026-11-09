package ptycheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This same end-to-end workflow runs on Unix PTYs and Windows ConPTY.
func TestTasksApp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	bin := buildExample(t, "tasks")
	env := append(cleanEnv("TERM=xterm-256color", "COLORTERM=truecolor"), "OPENTUI_TASKS_FILE="+path)
	s := startEnv(t, bin, 80, 24, env)
	s.waitText("0 tasks · 0 done · 0 visible")
	s.send("a")
	s.send("Write a real app\r")
	s.waitText("[ ] Write a real app")
	s.waitText("1 tasks · 0 done · 1 visible")
	s.send("a")
	s.send("Discard this title")
	s.waitText("Discard this title")
	s.send("\x1b") // cancel editor; escape is resolved after the read timeout
	s.waitFor("editor to cancel", func() bool {
		_, _, visible := s.screen.Find("Discard this title")
		return !visible
	})
	s.waitText("1 tasks · 0 done · 1 visible")
	s.send("a")
	s.send("Test the UI\r")
	s.waitText("[ ] Test the UI")
	s.send("e")
	s.send("\x15") // Ctrl+U clears the editor
	s.send("Test the keyboard\r")
	s.waitText("[ ] Test the keyboard")
	s.send("\r") // toggle the selected task
	s.waitText("[x] Test the keyboard")
	s.waitText("2 tasks · 1 done · 2 visible")
	s.send("/")
	s.send("keyboard")
	s.waitText("2 tasks · 1 done · 1 visible")
	if _, _, ok := s.screen.Find("Write a real app"); ok {
		t.Fatal("filter left an unmatched task visible")
	}
	s.send("\r") // filter input submits and focuses the list
	s.send("a")
	s.send("Third task\r") // save clears the filter to reveal the new task
	s.waitText("3 tasks · 1 done · 3 visible")
	s.waitText("[ ] Third task")
	s.resize(60, 20)
	s.waitFor("task list after resize", func() bool {
		_, _, found := s.screen.Find("[ ] Third task")
		lines := s.screen.Lines()
		return found && len(lines) == 20 && strings.HasSuffix(lines[0], "╮") && len([]rune(lines[0])) == 60
	})
	s.exitCleanly()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Write a real app", "Test the keyboard", "Third task"} {
		if !strings.Contains(string(b), title) {
			t.Fatalf("saved file missing %q: %s", title, b)
		}
	}
	if !strings.Contains(string(b), `"done": true`) {
		t.Fatalf("completion was not persisted: %s", b)
	}
	s = startEnv(t, bin, 80, 24, env)
	s.waitText("3 tasks · 1 done · 3 visible")
	s.waitText("[x] Test the keyboard")
	s.exitCleanly()
}
