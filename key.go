package opentui

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// KeyEventType distinguishes key presses, repeats, and releases. Traditional
// terminals report presses only; the Kitty keyboard protocol can report all
// three.
type KeyEventType uint8

const (
	KeyPress KeyEventType = iota
	KeyRepeat
	KeyRelease
)

// KeyEvent is one decoded terminal key. Name uses stable names such as
// "left", "return", "backspace", or the lower-case letter for alphabetic
// keys. Sequence is the exact input text for printable keys.
type KeyEvent struct {
	Name                            string
	Sequence                        string
	Raw                             string
	Ctrl, Alt, Meta, Option, Shift  bool
	Super, Hyper, CapsLock, NumLock bool
	Number                          bool
	Type                            KeyEventType
	defaultPrevented                bool
	propagationStopped              bool
}

// PreventDefault suppresses renderer defaults such as Ctrl+C exit and Tab
// focus traversal.
func (e *KeyEvent) PreventDefault() { e.defaultPrevented = true }

// DefaultPrevented reports whether PreventDefault was called.
func (e *KeyEvent) DefaultPrevented() bool { return e.defaultPrevented }

// StopPropagation prevents delivery to lower-priority handlers.
func (e *KeyEvent) StopPropagation() { e.propagationStopped = true }

// PropagationStopped reports whether StopPropagation was called.
func (e *KeyEvent) PropagationStopped() bool { return e.propagationStopped }

// PasteEvent is the content between bracketed-paste markers. Text is kept
// verbatim; controls and newlines are for the focused control to sanitize.
type PasteEvent struct {
	Text               string
	defaultPrevented   bool
	propagationStopped bool
}

func (e *PasteEvent) PreventDefault()          { e.defaultPrevented = true }
func (e *PasteEvent) DefaultPrevented() bool   { return e.defaultPrevented }
func (e *PasteEvent) StopPropagation()         { e.propagationStopped = true }
func (e *PasteEvent) PropagationStopped() bool { return e.propagationStopped }

type terminalEvent interface{ terminalEvent() }
type keyInput struct{ event *KeyEvent }
type pasteInput struct{ event *PasteEvent }

func (keyInput) terminalEvent()   {}
func (pasteInput) terminalEvent() {}

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// keyDecoder incrementally separates terminal input into complete keys. It
// retains partial UTF-8 and escape sequences across reads.
type keyDecoder struct {
	buf   []byte
	paste bool
}

func (d *keyDecoder) Feed(data []byte) []terminalEvent {
	d.buf = append(d.buf, data...)
	return d.decode(false)
}

// Flush resolves an ambiguous lone Escape after the input timeout.
func (d *keyDecoder) Flush() []terminalEvent { return d.decode(true) }

func (d *keyDecoder) decode(flush bool) []terminalEvent {
	var out []terminalEvent
	for len(d.buf) > 0 {
		if d.paste {
			if end := bytes.Index(d.buf, []byte(pasteEnd)); end >= 0 {
				out = append(out, pasteInput{&PasteEvent{Text: string(d.buf[:end])}})
				d.buf = d.buf[end+len(pasteEnd):]
				d.paste = false
				continue
			}
			// Keep enough trailing bytes to recognize an end marker split
			// across reads. Paste is emitted only when the marker is complete.
			return out
		}

		if bytes.HasPrefix(d.buf, []byte(pasteStart)) {
			d.buf = d.buf[len(pasteStart):]
			d.paste = true
			continue
		}

		n, complete := nextSequence(d.buf, flush)
		if !complete {
			return out
		}
		raw := string(d.buf[:n])
		d.buf = d.buf[n:]
		if key := parseKey(raw); key != nil {
			out = append(out, keyInput{key})
		}
	}
	return out
}

func nextSequence(b []byte, flush bool) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	if b[0] != 0x1b {
		if b[0] < utf8.RuneSelf {
			return 1, true
		}
		if !utf8.FullRune(b) {
			return 0, false
		}
		_, n := utf8.DecodeRune(b)
		return n, true
	}
	if len(b) == 1 {
		return 1, flush
	}
	if b[1] == '[' {
		if len(b) >= 3 && b[2] == 'M' {
			if len(b) >= 6 {
				return 6, true
			}
			return len(b), flush
		}
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				return i + 1, true
			}
		}
		return len(b), flush
	}
	if b[1] == ']' { // OSC response, terminated by BEL or ST (ESC backslash).
		for i := 2; i < len(b); i++ {
			if b[i] == 0x07 {
				return i + 1, true
			}
			if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
				return i + 2, true
			}
		}
		return len(b), flush
	}
	if b[1] == 'O' {
		if len(b) >= 3 {
			return 3, true
		}
		return len(b), flush
	}
	// ESC + rune is an Alt/Meta key.
	if !utf8.FullRune(b[1:]) {
		return 0, false
	}
	_, n := utf8.DecodeRune(b[1:])
	return 1 + n, true
}

var simpleCSI = map[string]string{
	"A": "up", "B": "down", "C": "right", "D": "left",
	"E": "clear", "F": "end", "H": "home", "Z": "tab",
	"1~": "home", "2~": "insert", "3~": "delete", "4~": "end",
	"5~": "pageup", "6~": "pagedown", "7~": "home", "8~": "end",
	"11~": "f1", "12~": "f2", "13~": "f3", "14~": "f4",
	"15~": "f5", "17~": "f6", "18~": "f7", "19~": "f8",
	"20~": "f9", "21~": "f10", "23~": "f11", "24~": "f12",
}

var ss3 = map[byte]string{
	'A': "up", 'B': "down", 'C': "right", 'D': "left", 'F': "end", 'H': "home",
	'P': "f1", 'Q': "f2", 'R': "f3", 'S': "f4", 'M': "return",
	'p': "0", 'q': "1", 'r': "2", 's': "3", 't': "4", 'u': "5",
	'v': "6", 'w': "7", 'x': "8", 'y': "9", 'j': "*", 'k': "+",
	'l': ",", 'm': "-", 'n': ".", 'o': "/", 'X': "=",
}

func parseKey(raw string) *KeyEvent {
	e := &KeyEvent{Raw: raw, Sequence: raw, Type: KeyPress}
	b := []byte(raw)
	if len(b) == 0 {
		return nil
	}

	if len(b) == 1 {
		switch b[0] {
		case '\r':
			e.Name = "return"
		case '\n':
			e.Name = "linefeed"
		case '\t':
			e.Name = "tab"
		case '\b', 0x7f:
			e.Name = "backspace"
		case 0x1b:
			e.Name = "escape"
		case ' ':
			e.Name = "space"
		case 0:
			e.Name, e.Ctrl = "space", true
		default:
			if b[0] >= 1 && b[0] <= 26 {
				e.Name, e.Ctrl = string(rune('a'+b[0]-1)), true
			} else if b[0] >= 28 && b[0] <= 31 {
				e.Name, e.Ctrl = string(rune(b[0]+64)), true
			} else {
				setPrintable(e, raw)
			}
		}
		return e
	}

	if strings.HasPrefix(raw, "\x1b[") {
		return parseCSI(e, raw[2:])
	}
	if strings.HasPrefix(raw, "\x1b]") {
		return nil
	}
	if len(b) == 3 && b[0] == 0x1b && b[1] == 'O' {
		e.Name = ss3[b[2]]
		if len(e.Name) == 1 {
			e.Sequence = e.Name
			e.Number = e.Name >= "0" && e.Name <= "9"
		}
		if e.Name == "" {
			return nil
		}
		return e
	}
	if b[0] == 0x1b {
		e.Alt, e.Meta, e.Option = true, true, true
		inner := parseKey(raw[1:])
		if inner == nil {
			return nil
		}
		e.Name, e.Ctrl, e.Shift, e.Number = inner.Name, inner.Ctrl, inner.Shift, inner.Number
		return e
	}
	setPrintable(e, raw)
	return e
}

func setPrintable(e *KeyEvent, s string) {
	r, _ := utf8.DecodeRuneInString(s)
	e.Name = s
	if unicode.IsUpper(r) {
		e.Name = string(unicode.ToLower(r))
		e.Shift = true
	}
	e.Number = len(s) == 1 && s[0] >= '0' && s[0] <= '9'
}

func parseCSI(e *KeyEvent, body string) *KeyEvent {
	if body == "" {
		return nil
	}
	// Filter reports and mouse/focus sequences that are not keyboard input.
	if strings.HasPrefix(body, "<") || strings.HasPrefix(body, "M") || body == "I" || body == "O" ||
		strings.HasSuffix(body, "R") || strings.HasSuffix(body, "t") ||
		strings.HasSuffix(body, "c") || strings.HasSuffix(body, "$y") {
		return nil
	}
	if name := simpleCSI[body]; name != "" {
		e.Name = name
		if body == "Z" {
			e.Shift = true
		}
		return e
	}

	final := body[len(body)-1]
	params := body[:len(body)-1]
	parts := strings.Split(params, ";")
	if final == 'u' { // Kitty keyboard protocol / CSI-u.
		codes := strings.Split(parts[0], ":")
		code, err := strconv.Atoi(codes[0])
		if err != nil {
			return nil
		}
		mod, event := 1, 1
		if len(parts) > 1 {
			mp := strings.Split(parts[1], ":")
			mod, _ = strconv.Atoi(mp[0])
			if len(mp) > 1 {
				event, _ = strconv.Atoi(mp[1])
			}
		}
		applyModifier(e, mod)
		switch event {
		case 2:
			e.Type = KeyRepeat
		case 3:
			e.Type = KeyRelease
		}
		setCodepointName(e, code)
		if len(parts) > 2 {
			var text strings.Builder
			for _, value := range strings.Split(parts[2], ":") {
				cp, err := strconv.Atoi(value)
				if err == nil && cp > 0 && cp <= utf8.MaxRune {
					text.WriteRune(rune(cp))
				}
			}
			if text.Len() > 0 {
				e.Sequence = text.String()
				if code == 0 {
					e.Name = text.String()
				}
			}
		} else if e.Shift && len(codes) > 1 {
			shifted, err := strconv.Atoi(codes[1])
			if err == nil && shifted > 0 && shifted <= utf8.MaxRune {
				e.Sequence = string(rune(shifted))
			}
		}
		if e.Name == "" {
			return nil
		}
		return e
	}

	if final == '~' && len(parts) >= 3 && parts[0] == "27" { // modifyOtherKeys
		mod, _ := strconv.Atoi(parts[1])
		code, _ := strconv.Atoi(parts[2])
		applyModifier(e, mod)
		setCodepointName(e, code)
		return e
	}

	if strings.ContainsRune("ABCDEFHPQS", rune(final)) {
		name := map[byte]string{
			'A': "up", 'B': "down", 'C': "right", 'D': "left", 'E': "clear",
			'F': "end", 'H': "home", 'P': "f1", 'Q': "f2", 'S': "f4",
		}[final]
		e.Name = name
		if len(parts) > 1 {
			modEvent := strings.Split(parts[len(parts)-1], ":")
			mod, _ := strconv.Atoi(modEvent[0])
			applyModifier(e, mod)
			if len(modEvent) > 1 {
				setEventType(e, modEvent[1])
			}
		}
		return e
	}
	if final == '~' {
		base := parts[0] + "~"
		e.Name = simpleCSI[base]
		if len(parts) > 1 {
			modEvent := strings.Split(parts[1], ":")
			mod, _ := strconv.Atoi(modEvent[0])
			applyModifier(e, mod)
			if len(modEvent) > 1 {
				setEventType(e, modEvent[1])
			}
		}
		if e.Name != "" {
			return e
		}
	}
	return nil
}

// Terminal modifiers are encoded as one plus a bitset: Shift=1, Alt=2,
// Ctrl=4, Super=8, Hyper=16.
func applyModifier(e *KeyEvent, encoded int) {
	m := max(encoded-1, 0)
	e.Shift, e.Alt, e.Ctrl = m&1 != 0, m&2 != 0, m&4 != 0
	e.Super, e.Hyper = m&8 != 0, m&16 != 0
	e.Meta, e.Option = e.Alt || m&32 != 0, e.Alt
	e.CapsLock, e.NumLock = m&64 != 0, m&128 != 0
}

func setCodepointName(e *KeyEvent, code int) {
	if name := kittyKeyName(code); name != "" {
		e.Name = name
		if text := kittyKeypadText(name); text != "" {
			e.Sequence = text
			e.Number = text >= "0" && text <= "9"
		}
		return
	}
	switch code {
	case 13:
		e.Name = "return"
	case 10:
		e.Name = "linefeed"
	case 9:
		e.Name = "tab"
	case 27:
		e.Name = "escape"
	case 32:
		e.Name, e.Sequence = "space", " "
	case 8, 127:
		e.Name = "backspace"
	default:
		if code > 0 && code <= utf8.MaxRune {
			s := string(rune(code))
			e.Sequence = s
			setPrintable(e, s)
		}
	}
}

func setEventType(e *KeyEvent, value string) {
	switch value {
	case "2":
		e.Type = KeyRepeat
	case "3":
		e.Type = KeyRelease
	}
}

func kittyKeyName(code int) string {
	base := [...]string{"escape", "return", "tab", "backspace", "insert", "delete", "left", "right", "up", "down", "pageup", "pagedown", "home", "end", "capslock", "scrolllock", "numlock", "printscreen", "pause", "menu"}
	if code >= 57344 && code < 57344+len(base) {
		return base[code-57344]
	}
	if code >= 57364 && code <= 57398 {
		return "f" + strconv.Itoa(code-57363)
	}
	keypad := [...]string{"kp0", "kp1", "kp2", "kp3", "kp4", "kp5", "kp6", "kp7", "kp8", "kp9", "kpdecimal", "kpdivide", "kpmultiply", "kpminus", "kpplus", "kpenter", "kpequal", "kpseparator", "kpleft", "kpright", "kpup", "kpdown", "kppageup", "kppagedown", "kphome", "kpend", "kpinsert", "kpdelete", "clear"}
	if code >= 57399 && code < 57399+len(keypad) {
		return keypad[code-57399]
	}
	extra := [...]string{
		"mediaplay", "mediapause", "mediaplaypause", "mediareverse", "mediastop",
		"mediafastforward", "mediarewind", "medianext", "mediaprev", "mediarecord",
		"volumedown", "volumeup", "mute", "leftshift", "leftctrl", "leftalt",
		"leftsuper", "lefthyper", "leftmeta", "rightshift", "rightctrl", "rightalt",
		"rightsuper", "righthyper", "rightmeta", "iso_level3_shift", "iso_level5_shift",
	}
	if code >= 57428 && code < 57428+len(extra) {
		return extra[code-57428]
	}
	return ""
}

func kittyKeypadText(name string) string {
	return map[string]string{"kp0": "0", "kp1": "1", "kp2": "2", "kp3": "3", "kp4": "4", "kp5": "5", "kp6": "6", "kp7": "7", "kp8": "8", "kp9": "9", "kpdecimal": ".", "kpdivide": "/", "kpmultiply": "*", "kpminus": "-", "kpplus": "+", "kpequal": "=", "kpseparator": ","}[name]
}
