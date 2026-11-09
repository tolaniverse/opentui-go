// Package ansi holds the raw terminal escape sequences OpenTUI-Go writes.
//
// Port of the `ANSI` struct in upstream packages/native/src/ansi.zig.
// Only the sequences needed by v0.1.0 are included.
package ansi

import (
	"encoding/base64"
	"io"
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
// both satisfy it. Sequences are written with WriteByte, WriteRune and
// WriteString only: a byte slice passed through this interface counts as
// escaping, so building sequences in a local array would allocate on
// every call.
type Writer interface {
	io.Writer
	WriteString(s string) (int, error)
	WriteByte(c byte) error
	WriteRune(r rune) (int, error)
}

// writeDec writes a non-negative decimal number one byte at a time.
func writeDec(w Writer, n int) {
	if n >= 10 {
		writeDec(w, n/10)
	}
	w.WriteByte(byte('0' + n%10))
}

// MoveTo writes CUP (cursor position). x and y are 1-based, like the terminal.
func MoveTo(w Writer, x, y int) {
	w.WriteString("\x1b[")
	writeDec(w, y)
	w.WriteByte(';')
	writeDec(w, x)
	w.WriteByte('H')
}

// FgRGB writes a 24-bit "truecolor" foreground: CSI 38;2;r;g;b m.
func FgRGB(w Writer, r, g, b uint8) { sgrRGB(w, "\x1b[38;2;", r, g, b) }

// BgRGB writes a 24-bit background: CSI 48;2;r;g;b m.
func BgRGB(w Writer, r, g, b uint8) { sgrRGB(w, "\x1b[48;2;", r, g, b) }

// FgIndexed writes a 256-palette foreground: CSI 38;5;n m.
func FgIndexed(w Writer, n uint8) { sgrIndexed(w, "\x1b[38;5;", n) }

// BgIndexed writes a 256-palette background: CSI 48;5;n m.
func BgIndexed(w Writer, n uint8) { sgrIndexed(w, "\x1b[48;5;", n) }

func sgrRGB(w Writer, prefix string, r, g, b uint8) {
	w.WriteString(prefix)
	writeDec(w, int(r))
	w.WriteByte(';')
	writeDec(w, int(g))
	w.WriteByte(';')
	writeDec(w, int(b))
	w.WriteByte('m')
}

func sgrIndexed(w Writer, prefix string, n uint8) {
	w.WriteString(prefix)
	writeDec(w, int(n))
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

// Basic writes one of the 16 basic colors (index 0-15) with the original
// SGR codes: 30-37 / 90-97 for the foreground, 40-47 / 100-107 for the
// background. Consoles without 256-color support understand only these.
func Basic(w Writer, index uint8, background bool) {
	code := 30 + int(index)
	if index >= 8 {
		code = 90 + int(index) - 8
	}
	if background {
		code += 10
	}
	w.WriteString("\x1b[")
	writeDec(w, code)
	w.WriteByte('m')
}

// LinkEnd closes an OSC 8 hyperlink.
const LinkEnd = "\x1b]8;;\x1b\\"

// LinkStart opens an OSC 8 hyperlink to url, as upstream renderer.zig:
// ESC ] 8 ; id=N ; url ST. The id lets terminals treat the pieces of one
// link that are split across rows or runs as a single link.
func LinkStart(w Writer, id int, url string) {
	w.WriteString("\x1b]8;id=")
	writeDec(w, id)
	w.WriteByte(';')
	w.WriteString(url)
	w.WriteString("\x1b\\")
}
