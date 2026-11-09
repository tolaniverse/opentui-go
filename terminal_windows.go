//go:build windows

package opentui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// platformState holds the console output mode to restore on Close.
type platformState struct {
	outMode    uint32
	outModeSet bool
}

// ErrNoVirtualTerminal is returned on Windows consoles that cannot
// interpret escape sequences (before Windows 10 version 1511).
var ErrNoVirtualTerminal = errors.New("opentui: this console does not support virtual terminal sequences")

// enableTerminalOutput turns on escape-sequence processing for the output
// console. DISABLE_NEWLINE_AUTO_RETURN keeps "\n" from also returning the
// cursor, as on Unix terminals.
func (r *Renderer) enableTerminalOutput() error {
	h := windows.Handle(r.out.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return fmt.Errorf("opentui: console mode: %w", err)
	}
	vt := mode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.DISABLE_NEWLINE_AUTO_RETURN
	if err := windows.SetConsoleMode(h, vt); err != nil {
		// Older consoles reject DISABLE_NEWLINE_AUTO_RETURN; retry without it.
		if err := windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
			return ErrNoVirtualTerminal
		}
	}
	r.platform = platformState{outMode: mode, outModeSet: true}
	return nil
}

func (r *Renderer) restoreTerminalOutput() error {
	if !r.platform.outModeSet {
		return nil
	}
	return windows.SetConsoleMode(windows.Handle(r.out.Fd()), r.platform.outMode)
}

// resizePollInterval is how often the console size is checked: Windows has
// no resize signal.
const resizePollInterval = 100 * time.Millisecond

// notifySignals delivers Ctrl+Break/close (os.Interrupt, SIGTERM) and, by
// polling the console size, resizes on sigs. Ctrl+C itself arrives as a
// key: raw mode turns off processed input.
func (r *Renderer) notifySignals(ctx context.Context, sigs chan os.Signal) (stop func()) {
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	pollCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		w, h, _ := term.GetSize(int(r.out.Fd()))
		ticker := time.NewTicker(resizePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				nw, nh, err := term.GetSize(int(r.out.Fd()))
				if err != nil || (nw == w && nh == h) {
					continue
				}
				w, h = nw, nh
				select {
				case sigs <- resizeSignal{}:
				default: // a resize is already pending; Run reads the size itself
				}
			}
		}
	}()
	return func() {
		signal.Stop(sigs)
		cancel()
		<-done
	}
}

func isResizeSignal(s os.Signal) bool { return s == resizeSignal{} }

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procPeekConsoleInputW = kernel32.NewProc("PeekConsoleInputW")
	procReadConsoleInputW = kernel32.NewProc("ReadConsoleInputW")
)

// inputRecord is the Win32 INPUT_RECORD; for KEY_EVENT, the event bytes
// are a KEY_EVENT_RECORD.
type inputRecord struct {
	eventType uint16
	_         uint16
	event     [16]byte
}

// keyDownWithChar reports whether rec is a key press that produces input
// characters. In virtual-terminal input mode, arrows, function keys and
// mouse reports arrive as runs of such records.
func (rec *inputRecord) keyDownWithChar() bool {
	if rec.eventType != windows.KEY_EVENT {
		return false
	}
	keyDown := *(*int32)(unsafe.Pointer(&rec.event[0]))
	char := *(*uint16)(unsafe.Pointer(&rec.event[10]))
	return keyDown != 0 && char != 0
}

func consoleInput(proc *windows.LazyProc, h windows.Handle, recs []inputRecord) (int, error) {
	var n uint32
	r1, _, err := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&recs[0])), uintptr(len(recs)), uintptr(unsafe.Pointer(&n)))
	if r1 == 0 {
		return 0, err
	}
	return int(n), nil
}

// readInput decodes console input. A console read blocks until a key that
// produces characters arrives, so it is called only when one is pending;
// other records (focus, resize, menu, bare modifier keys) are discarded
// first. The 50 ms wait lets cancellation and a lone Escape be noticed.
func (r *Renderer) readInput(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	h := windows.Handle(r.in.Fd())
	buf := make([]byte, 256)
	recs := make([]inputRecord, 64)
	decoder := keyDecoder{}
	for ctx.Err() == nil {
		event, err := windows.WaitForSingleObject(h, 50)
		if err != nil {
			return
		}
		if event == uint32(windows.WAIT_TIMEOUT) {
			if !r.emitInput(ctx, decoder.Flush()) {
				return
			}
			continue
		}
		n, err := consoleInput(procPeekConsoleInputW, h, recs)
		if err != nil {
			return
		}
		skip := 0
		for skip < n && !recs[skip].keyDownWithChar() {
			skip++
		}
		if skip > 0 {
			if _, err := consoleInput(procReadConsoleInputW, h, recs[:skip]); err != nil {
				return
			}
			continue
		}
		if n == 0 {
			continue
		}
		m, err := r.in.Read(buf)
		if err != nil || m == 0 {
			return
		}
		if !r.emitInput(ctx, decoder.Feed(buf[:m])) {
			return
		}
	}
}
