package opentui

import "testing"

func keys(events []terminalEvent) []*KeyEvent {
	var out []*KeyEvent
	for _, event := range events {
		if k, ok := event.(keyInput); ok {
			out = append(out, k.event)
		}
	}
	return out
}

func TestKeyDecoderBasicAndModifiers(t *testing.T) {
	tests := []struct {
		raw, name        string
		ctrl, alt, shift bool
	}{
		{"a", "a", false, false, false},
		{"A", "a", false, false, true},
		{"\x01", "a", true, false, false},
		{"\x00", "space", true, false, false},
		{"\x1bx", "x", false, true, false},
		{"\x1b[A", "up", false, false, false},
		{"\x1b[1;5D", "left", true, false, false},
		{"\x1b[Z", "tab", false, false, true},
		{"\x1b[3~", "delete", false, false, false},
		{"\x1bOP", "f1", false, false, false},
		{"\x1b[27;6;13~", "return", true, false, true},
	}
	for _, tc := range tests {
		d := keyDecoder{}
		got := keys(d.Feed([]byte(tc.raw)))
		if len(got) != 1 {
			t.Fatalf("%q: got %d events", tc.raw, len(got))
		}
		k := got[0]
		if k.Name != tc.name || k.Ctrl != tc.ctrl || k.Alt != tc.alt || k.Shift != tc.shift {
			t.Errorf("%q: got name=%q ctrl=%v alt=%v shift=%v", tc.raw, k.Name, k.Ctrl, k.Alt, k.Shift)
		}
	}
}

func TestKeyDecoderSplitSequencesAndUTF8(t *testing.T) {
	d := keyDecoder{}
	if got := d.Feed([]byte("\x1b[1;")); len(got) != 0 {
		t.Fatalf("partial CSI emitted: %v", got)
	}
	got := keys(d.Feed([]byte("5C")))
	if len(got) != 1 || got[0].Name != "right" || !got[0].Ctrl {
		t.Fatalf("split CSI: %+v", got)
	}

	d = keyDecoder{}
	b := []byte("é")
	if got := d.Feed(b[:1]); len(got) != 0 {
		t.Fatal("partial UTF-8 emitted")
	}
	got = keys(d.Feed(b[1:]))
	if len(got) != 1 || got[0].Name != "é" || got[0].Sequence != "é" {
		t.Fatalf("UTF-8: %+v", got)
	}
}

func TestKeyDecoderEscapeTimeout(t *testing.T) {
	d := keyDecoder{}
	if got := d.Feed([]byte{0x1b}); len(got) != 0 {
		t.Fatal("lone escape must wait")
	}
	got := keys(d.Flush())
	if len(got) != 1 || got[0].Name != "escape" {
		t.Fatalf("flush: %+v", got)
	}
}

func TestKeyDecoderBracketedPasteAcrossReads(t *testing.T) {
	d := keyDecoder{}
	if got := d.Feed([]byte("\x1b[200~hello\nwor")); len(got) != 0 {
		t.Fatal("paste emitted before end marker")
	}
	got := d.Feed([]byte("ld\x1b[201~x"))
	if len(got) != 2 {
		t.Fatalf("got %d events", len(got))
	}
	p, ok := got[0].(pasteInput)
	if !ok || p.event.Text != "hello\nworld" {
		t.Fatalf("paste: %#v", got[0])
	}
	k, ok := got[1].(keyInput)
	if !ok || k.event.Name != "x" {
		t.Fatalf("trailing key: %#v", got[1])
	}
}

func TestKeyDecoderKittyPressRepeatRelease(t *testing.T) {
	for _, tc := range []struct {
		raw string
		typ KeyEventType
	}{
		{"\x1b[97;1u", KeyPress}, {"\x1b[97;1:2u", KeyRepeat}, {"\x1b[97;1:3u", KeyRelease},
	} {
		got := keys((&keyDecoder{}).Feed([]byte(tc.raw)))
		if len(got) != 1 || got[0].Name != "a" || got[0].Type != tc.typ {
			t.Errorf("%q: %+v", tc.raw, got)
		}
	}
}

func TestKeyDecoderKittyNamedKeysAndLocks(t *testing.T) {
	got := keys((&keyDecoder{}).Feed([]byte("\x1b[57350;207u")))
	if len(got) != 1 {
		t.Fatalf("got %d keys", len(got))
	}
	k := got[0]
	if k.Name != "left" || !k.Ctrl || !k.Alt || !k.Meta || !k.Option || !k.Super || !k.CapsLock || !k.NumLock {
		t.Fatalf("named key/modifiers: %+v", k)
	}
	got = keys((&keyDecoder{}).Feed([]byte("\x1b[57440u")))
	if len(got) != 1 || got[0].Name != "mute" {
		t.Fatalf("media key: %+v", got)
	}
	got = keys((&keyDecoder{}).Feed([]byte("\x1b[97;33u")))
	if len(got) != 1 || !got[0].Meta || got[0].Alt || got[0].Option {
		t.Fatalf("meta modifier: %+v", got)
	}
}

func TestKeyDecoderKittyAssociatedText(t *testing.T) {
	got := keys((&keyDecoder{}).Feed([]byte("\x1b[0;1;104:105u")))
	if len(got) != 1 || got[0].Name != "hi" || got[0].Sequence != "hi" {
		t.Fatalf("associated text: %+v", got)
	}
	got = keys((&keyDecoder{}).Feed([]byte("\x1b[97:65;2u")))
	if len(got) != 1 || got[0].Name != "a" || got[0].Sequence != "A" || !got[0].Shift {
		t.Fatalf("shifted text: %+v", got)
	}
}

func TestKeyDecoderKittySpecialEventTypes(t *testing.T) {
	for _, tc := range []struct {
		raw string
		typ KeyEventType
	}{
		{"\x1b[1;1:2A", KeyRepeat},
		{"\x1b[3;1:3~", KeyRelease},
		{"\x1b[1;1:2P", KeyRepeat},
	} {
		got := keys((&keyDecoder{}).Feed([]byte(tc.raw)))
		if len(got) != 1 || got[0].Type != tc.typ {
			t.Errorf("%q: %+v", tc.raw, got)
		}
	}
}

func TestKeyDecoderSuppressesOSCAndIncompleteCSI(t *testing.T) {
	d := keyDecoder{}
	inputs := [][]byte{
		[]byte("\x1b]11;rgb:ffff/ffff/ffff\x07"),
		[]byte("\x1b]52;c;ignored\x1b\\"),
	}
	for _, input := range inputs {
		if got := d.Feed(input); len(got) != 0 {
			t.Fatalf("control input emitted: %#v", got)
		}
	}
	d.Feed([]byte("\x1b["))
	if got := d.Flush(); len(got) != 0 {
		t.Fatalf("incomplete CSI emitted: %#v", got)
	}
}
