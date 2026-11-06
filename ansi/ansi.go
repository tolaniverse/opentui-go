// Package ansi holds the raw terminal escape sequences OpenTUI-Go writes.
//
// Port of the `ANSI` struct in upstream packages/native/src/ansi.zig.
// Only the sequences needed by v0.1.0 are included.
package ansi

import (
	"encoding/base64"
	"io"
	"strconv"
)

// Fixed sequences. CSI = "\x1b[".
const (
	Reset              = "\x1b[0m"
	Clear              = "\x1b[2J"
	Home               = "\x1b[H"
	HideCursor         = "\x1b[?25l"
	ShowCursor         = "\x1b[?25h"
	AltScreenEnter     = "\x1b[?1049h"
	AltScreenExit      = "\x1b[?1049l"
	SyncBegin          = "\x1b[?2026h"
	SyncEnd            = "\x1b[?2026l"
	BracketedPasteOn   = "\x1b[?2004h"
	BracketedPasteOff  = "\x1b[?2004l"
	MouseMotionOn      = "\x1b[?1003h"
	MouseMotionOff     = "\x1b[?1003l"
	MouseSGROn         = "\x1b[?1006h"
	MouseSGROff        = "\x1b[?1006l"
	FgDefault          = "\x1b[39m"
	BgDefault          = "\x1b[49m"
	Bold               = "\x1b[1m"
	Dim                = "\x1b[2m"
	Italic             = "\x1b[3m"
	Underline          = "\x1b[4m"
	Blink              = "\x1b[5m"
	Inverse            = "\x1b[7m"
	Hidden             = "\x1b[8m"
	Strikethrough      = "\x1b[9m"
	DefaultCursorStyle = "\x1b[0 q"
)

// Writer is the minimal interface we need. *bufio.Writer and *bytes.Buffer
// both satisfy it, so sequences can be built without allocating strings.
type Writer interface {
	io.Writer
	WriteString(s string) (int, error)
	WriteByte(c byte) error
}

func writeInt(w Writer, n int) {
	var tmp [20]byte
	w.Write(strconv.AppendInt(tmp[:0], int64(n), 10))
}

// MoveTo writes CUP (cursor position). x and y are 1-based, like the terminal.
func MoveTo(w Writer, x, y int) {
	w.WriteString("\x1b[")
	writeInt(w, y)
	w.WriteByte(';')
	writeInt(w, x)
	w.WriteByte('H')
}

// FgRGB writes a 24-bit "truecolor" foreground: CSI 38;2;r;g;b m.
func FgRGB(w Writer, r, g, b uint8) { sgrRGB(w, 38, r, g, b) }

// BgRGB writes a 24-bit background: CSI 48;2;r;g;b m.
func BgRGB(w Writer, r, g, b uint8) { sgrRGB(w, 48, r, g, b) }

// FgIndexed writes a 256-palette foreground: CSI 38;5;n m.
func FgIndexed(w Writer, n uint8) { sgrIndexed(w, 38, n) }

// BgIndexed writes a 256-palette background: CSI 48;5;n m.
func BgIndexed(w Writer, n uint8) { sgrIndexed(w, 48, n) }

func sgrRGB(w Writer, code int, r, g, b uint8) {
	w.WriteString("\x1b[")
	writeInt(w, code)
	w.WriteString(";2;")
	writeInt(w, int(r))
	w.WriteByte(';')
	writeInt(w, int(g))
	w.WriteByte(';')
	writeInt(w, int(b))
	w.WriteByte('m')
}

func sgrIndexed(w Writer, code int, n uint8) {
	w.WriteString("\x1b[")
	writeInt(w, code)
	w.WriteString(";5;")
	writeInt(w, int(n))
	w.WriteByte('m')
}

// SetTitle writes OSC 0 (window/icon title).
func SetTitle(w Writer, title string) {
	w.WriteString("\x1b]0;")
	w.WriteString(title)
	w.WriteByte('\x07')
}

// Clipboard writes an OSC 52 request asking the terminal to place text on
// a clipboard. target is 'c' (clipboard) or 'p' (primary selection). The
// payload is base64 and the sequence ends with ST, as upstream terminal.zig.
func Clipboard(w Writer, target byte, text string) {
	w.WriteString("\x1b]52;")
	w.WriteString(string(target))
	w.WriteString(";")
	w.WriteString(base64.StdEncoding.EncodeToString([]byte(text)))
	w.WriteString("\x1b\\")
}
