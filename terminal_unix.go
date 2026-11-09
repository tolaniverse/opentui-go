//go:build unix

package opentui

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// platformState is empty on Unix: raw mode (x/term) is all the terminal
// needs, and escape sequences are always interpreted.
type platformState struct{}

func (r *Renderer) enableTerminalOutput() error  { return nil }
func (r *Renderer) restoreTerminalOutput() error { return nil }

// notifySignals delivers SIGWINCH (resize) and the quit signals on sigs and
// returns a function that stops delivery.
func (r *Renderer) notifySignals(ctx context.Context, sigs chan os.Signal) (stop func()) {
	signal.Notify(sigs, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return func() { signal.Stop(sigs) }
}

func isResizeSignal(s os.Signal) bool { return s == syscall.SIGWINCH || s == resizeSignal{} }

// readInput decodes terminal input. It polls with a timeout both to notice
// cancellation and to distinguish a lone Escape from an Alt-prefixed key.
func (r *Renderer) readInput(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	fd := int(r.in.Fd())
	buf := make([]byte, 256)
	decoder := keyDecoder{}
	for ctx.Err() == nil {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int((50 * time.Millisecond).Milliseconds()))
		if err != nil {
			if errors.Is(err, unix.EINTR) { // a signal (e.g. SIGWINCH) interrupted the wait
				continue
			}
			return
		}
		if n == 0 {
			if !r.emitInput(ctx, decoder.Flush()) {
				return
			}
			continue
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			continue
		}
		m, err := unix.Read(fd, buf)
		if err != nil || m == 0 {
			return
		}
		if !r.emitInput(ctx, decoder.Feed(buf[:m])) {
			return
		}
	}
}
