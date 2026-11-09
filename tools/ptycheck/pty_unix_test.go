//go:build unix

package ptycheck

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// unixPTY runs a program on a Unix pseudo-terminal.
type unixPTY struct {
	cmd *exec.Cmd
	*os.File
}

func startTerminal(bin string, env []string, w, h int) (terminal, error) {
	cmd := exec.Command(bin)
	cmd.Env = env
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		return nil, err
	}
	return &unixPTY{cmd, tty}, nil
}

func (p *unixPTY) Resize(w, h int) error {
	return pty.Setsize(p.File, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
}

func (p *unixPTY) Kill()       { p.cmd.Process.Kill() }
func (p *unixPTY) Wait() error { return p.cmd.Wait() }
