package opentui

import (
	"errors"
	"strings"

	"github.com/tolaniverse/opentui-go/ansi"
)

// ClipboardTarget selects which terminal clipboard receives a copy.
type ClipboardTarget uint8

const (
	// ClipboardSystem is the regular system clipboard (OSC 52 target "c").
	ClipboardSystem ClipboardTarget = iota
	// ClipboardPrimary is the X11-style primary selection (target "p").
	ClipboardPrimary
)

// ErrClipboardText is returned for empty text or text containing NUL,
// which upstream also rejects before writing.
var ErrClipboardText = errors.New("opentui: clipboard text must be non-empty and contain no NUL")

// CopyToClipboard asks the terminal to copy text using OSC 52. This works
// over SSH because the terminal, not this process, owns the clipboard.
// Many terminals disable OSC 52 or limit its size, and none confirm it, so
// a nil error only means the request was written. Call it on the Run
// goroutine (a handler, draw callback, or Update func).
func (r *Renderer) CopyToClipboard(text string, target ClipboardTarget) error {
	if text == "" || strings.ContainsRune(text, 0) {
		return ErrClipboardText
	}
	code := byte('c')
	if target == ClipboardPrimary {
		code = 'p'
	}
	ansi.Clipboard(r.w, code, text)
	return r.w.Flush()
}
