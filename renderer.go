//go:build unix

package opentui

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/tolaniverse/opentui-go/ansi"
)

// Options configures a Renderer. The zero value is a sensible default.
type Options struct {
	In  *os.File // keyboard input; default os.Stdin
	Out *os.File // screen output; default os.Stdout

	// Background is painted under everything. Transparent (the zero value)
	// means "use the terminal's own background".
	Background RGBA

	// IgnoreCtrlC disables the default "Ctrl+C stops Run" behaviour.
	// (Upstream: exitOnCtrlC, default true.)
	IgnoreCtrlC bool

	// MainScreen draws on the normal screen instead of the alternate screen.
	// The alternate screen is a separate page that disappears on exit, so
	// the user's shell history is left untouched.
	MainScreen bool

	// NoSync disables synchronized output (DEC mode 2026).
	NoSync bool

	// Title sets the terminal window title while running.
	Title string

	// NoMouse disables mouse reporting. Mouse input and click-to-focus are on
	// by default, matching upstream OpenTUI.
	NoMouse bool
	// NoAutoFocus disables the default left-click focus behavior.
	NoAutoFocus bool

	// Color is how many colors to send. ColorAuto (the zero value) detects
	// it from TERM, COLORTERM and NO_COLOR; see DetectColorMode. Colors
	// beyond the mode are mapped to the nearest palette entry.
	Color ColorMode

	// Hyperlinks controls OSC 8 links from Span.Link. HyperlinksAuto (the
	// zero value) enables them only for terminals known to support them;
	// see DetectHyperlinks.
	Hyperlinks HyperlinkMode
}

// Renderer owns the terminal: it switches it into raw mode, keeps two
// buffers (what is on screen, what should be on screen) and paints the
// difference. Port of the core of `CliRenderer` (upstream renderer.ts and
// renderer.zig).
type Renderer struct {
	opts     Options
	colors   *colorEncoder
	in, out  *os.File
	w        *bufio.Writer
	oldState *term.State

	current *Buffer // what the terminal shows right now
	next    *Buffer // what the app is drawing for the next frame

	root                                   *Node // the UI tree, painted before the draw callback
	focused                                *Node
	keyHandler                             func(*KeyEvent)
	pasteHandler                           func(*PasteEvent)
	mouseHandler                           func(*MouseEvent)
	pressedMouse                           *Node
	capturedMouse                          *Node
	hoveredMouse                           *Node
	selection                              *Selection
	selectionHandler                       func(*Selection)
	lastClickNode                          *Node
	lastClickX, lastClickY, lastClickCount int
	lastClickTime                          time.Time
	autoScrolling                          bool

	renderReq   chan struct{}
	updates     chan func()
	inputEvents chan terminalEvent
	done        chan struct{} // closed when Run returns
	closeOnce   sync.Once
	closeErr    error
}

// ErrNotTerminal is returned when output is not a terminal (e.g. piped to a file).
var ErrNotTerminal = errors.New("opentui: output is not a terminal")

// New takes over the terminal. Always call Close (Run does it for you).
func New(opts Options) (*Renderer, error) {
	r := &Renderer{
		opts: opts, in: opts.In, out: opts.Out,
		root:        NewNode(Style{}),
		renderReq:   make(chan struct{}, 1),
		updates:     make(chan func(), 64),
		inputEvents: make(chan terminalEvent, 64),
		done:        make(chan struct{}),
	}
	r.root.ID = "root"
	mode := opts.Color
	if mode == ColorAuto {
		mode = DetectColorMode(os.Getenv)
	}
	r.colors = newColorEncoder(mode, opts.Background)
	switch opts.Hyperlinks {
	case HyperlinksOn:
		r.colors.links = true
	case HyperlinksAuto:
		r.colors.links = DetectHyperlinks(os.Getenv)
	}
	if r.in == nil {
		r.in = os.Stdin
	}
	if r.out == nil {
		r.out = os.Stdout
	}
	if !term.IsTerminal(int(r.out.Fd())) || !term.IsTerminal(int(r.in.Fd())) {
		return nil, ErrNotTerminal
	}

	width, height, err := term.GetSize(int(r.out.Fd()))
	if err != nil {
		return nil, err
	}

	// Raw mode: keys arrive one byte at a time, unechoed, and Ctrl+C is
	// delivered as byte 0x03 instead of killing the process.
	r.oldState, err = term.MakeRaw(int(r.in.Fd()))
	if err != nil {
		return nil, err
	}

	r.w = bufio.NewWriterSize(r.out, 64*1024)
	r.current = NewBuffer(width, height)
	r.next = NewBuffer(width, height)
	r.resetBuffers()

	if !opts.MainScreen {
		r.w.WriteString(ansi.AltScreenEnter)
	}
	r.w.WriteString(ansi.HideCursor)
	r.w.WriteString(ansi.BracketedPasteOn)
	if !opts.NoMouse {
		r.w.WriteString(ansi.MouseMotionOn)
		r.w.WriteString(ansi.MouseSGROn)
	}
	r.w.WriteString(ansi.Clear)
	if opts.Title != "" {
		ansi.SetTitle(r.w, opts.Title)
	}
	if err := r.w.Flush(); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

// resetBuffers marks the screen as unknown so the next frame repaints all
// cells, and clears the frame being drawn.
func (r *Renderer) resetBuffers() {
	r.current.fill(clearChar, RGB(0, 0, 0))
	bd := r.opts.Background
	bd.A = 255
	r.next.SetBackdrop(bd)
	r.next.Clear(r.opts.Background)
}

// Size returns the terminal size in cells.
func (r *Renderer) Size() (width, height int) { return r.next.Width(), r.next.Height() }

// Root is the top of the UI tree. Each frame it is sized to the terminal,
// laid out and painted, before the draw callback runs.
// Change the tree only on the Run goroutine (see Update).
func (r *Renderer) Root() *Node { return r.root }

// Update runs fn on the Run goroutine, then draws a frame. Use it to change
// the tree from other goroutines (timers, network, …). It returns false if
// Run has already finished, in which case fn is not run.
func (r *Renderer) Update(fn func()) bool {
	// Prefer the completed state when Update starts after Run has returned.
	// Without this fast path, both cases below can be ready and select may
	// enqueue work that no goroutine will ever apply.
	select {
	case <-r.done:
		return false
	default:
	}
	select {
	case r.updates <- fn:
		return true
	case <-r.done:
		return false
	}
}

// Buffer is the frame being drawn. Draw into it, then call Render.
func (r *Renderer) Buffer() *Buffer { return r.next }

// Render paints what changed since the last frame, then clears the drawing
// buffer for the next one.
func (r *Renderer) Render() error {
	encodeFrame(r.w, r.current, r.next, FrameOptions{Sync: !r.opts.NoSync, colors: r.colors})
	r.next.Clear(r.opts.Background)
	return r.w.Flush()
}

// Resize adapts both buffers to a new terminal size and schedules a full repaint.
func (r *Renderer) Resize(width, height int) {
	r.current.Resize(width, height)
	r.next.Resize(width, height)
	// No screen clear here: resetBuffers makes the next frame repaint every
	// cell inside one synchronized update, which avoids a visible flash.
	r.resetBuffers()
}

// RequestRender asks Run to draw a new frame. Safe to call from any
// goroutine; extra requests while one is pending are merged.
func (r *Renderer) RequestRender() {
	select {
	case r.renderReq <- struct{}{}:
	default:
	}
}

// Close restores the terminal exactly as we found it. Safe to call twice.
func (r *Renderer) Close() error {
	r.closeOnce.Do(func() {
		// Undo in reverse order of New.
		r.w.WriteString(ansi.Reset)
		if !r.opts.NoMouse {
			r.w.WriteString(ansi.MouseSGROff)
			r.w.WriteString(ansi.MouseMotionOff)
		}
		r.w.WriteString(ansi.BracketedPasteOff)
		r.w.WriteString(ansi.ShowCursor)
		r.w.WriteString(ansi.DefaultCursorStyle)
		if r.opts.Title != "" {
			ansi.SetTitle(r.w, "")
		}
		if !r.opts.MainScreen {
			r.w.WriteString(ansi.AltScreenExit)
		}
		flushErr := r.w.Flush()
		restoreErr := term.Restore(int(r.in.Fd()), r.oldState)
		r.closeErr = errors.Join(flushErr, restoreErr)
	})
	return r.closeErr
}

// Run draws frames until ctx is cancelled, Ctrl+C is pressed (unless
// IgnoreCtrlC), or the process gets SIGINT/SIGTERM/SIGHUP. draw is called
// for every frame after the Root tree is painted (draw may be nil); it runs
// on Run's goroutine.
//
// Frames are drawn at start, on resize, and on RequestRender.
// Run always calls Close, even if draw panics.
func (r *Renderer) Run(ctx context.Context, draw func(b *Buffer)) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.Close() // restore the terminal first so the panic is readable
			panic(p)
		}
		err = errors.Join(err, r.Close())
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)

	readerDone := make(chan struct{})
	defer func() { cancel(); <-readerDone }() // wait so no read outlives Run

	defer close(r.done)

	frame := func() error {
		r.validateFocus()
		r.drawFrame(r.next)
		if draw != nil {
			draw(r.next)
		}
		return r.Render()
	}
	if err := frame(); err != nil {
		return err
	}
	// Start reading only after mouse and paste modes are visible to the terminal.
	go r.readInput(ctx, readerDone)

	for {
		select {
		case <-ctx.Done():
			return nil
		case s := <-sigs:
			if s != syscall.SIGWINCH {
				return nil
			}
			w, h, err := term.GetSize(int(r.out.Fd()))
			if err != nil {
				return err
			}
			r.Resize(w, h)
			if err := frame(); err != nil {
				return err
			}
		case <-r.renderReq:
			if err := frame(); err != nil {
				return err
			}
		case fn := <-r.updates:
			fn()
			// Apply everything already queued before drawing once.
			for drained := false; !drained; {
				select {
				case fn := <-r.updates:
					fn()
				default:
					drained = true
				}
			}
			if err := frame(); err != nil {
				return err
			}
		case event := <-r.inputEvents:
			if r.dispatchInput(event) {
				return nil
			}
			if err := frame(); err != nil {
				return err
			}
		}
	}
}

// readInput decodes terminal input. It polls with a timeout both to notice
// cancellation and to distinguish a lone Escape from an Alt-prefixed key.
func (r *Renderer) readInput(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	fd := int(r.in.Fd())
	buf := make([]byte, 256)
	decoder := keyDecoder{}
	emit := func(events []terminalEvent) bool {
		for _, event := range events {
			select {
			case r.inputEvents <- event:
			case <-ctx.Done():
				return false
			}
		}
		return true
	}
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
			if !emit(decoder.Flush()) {
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
		if !emit(decoder.Feed(buf[:m])) {
			return
		}
	}
}

// ColorMode returns the color mode in use: Options.Color, or the mode
// detected from the environment when that was ColorAuto.
func (r *Renderer) ColorMode() ColorMode {
	if r.colors == nil {
		return ColorTrueColor
	}
	return r.colors.mode
}

// Hyperlinks reports whether OSC 8 hyperlinks are sent to the terminal.
func (r *Renderer) Hyperlinks() bool { return r.colors != nil && r.colors.links }
