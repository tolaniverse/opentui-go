package ptycheck

import "testing"

func TestScreenCursorTextAndColors(t *testing.T) {
	s := NewScreen(10, 3)
	s.Write([]byte("\x1b[2;3H\x1b[48;2;1;2;3mab\x1b[0mc"))
	if got := s.Lines()[1]; got != "  abc" {
		t.Fatalf("row = %q", got)
	}
	if c := s.Cell(2, 1); !c.BGSet || c.BG != [3]uint8{1, 2, 3} {
		t.Fatalf("colored cell = %+v", c)
	}
	if c := s.Cell(4, 1); c.BGSet {
		t.Fatalf("reset cell kept background: %+v", c)
	}
}

func TestScreenSplitSequencesModesAndOSC(t *testing.T) {
	s := NewScreen(10, 2)
	s.Write([]byte("\x1b[?1003;10"))
	s.Write([]byte("06h\x1b]0;tit"))
	s.Write([]byte("le\x07\x1b[1;1Hé"[:len("le\x07\x1b[1;1Hé")-1]))
	s.Write([]byte("é"[1:]))
	if !s.Mode(1003) || !s.Mode(1006) {
		t.Fatal("modes not set")
	}
	if got := s.Lines()[0]; got != "é" {
		t.Fatalf("row = %q", got)
	}
	s.Write([]byte("\x1b[?1003l"))
	if s.Mode(1003) {
		t.Fatal("mode not reset")
	}
}

// ConPTY re-encodes absolute cursor moves as relative ones. The emulator
// must follow those or Windows smoke tests look at the wrong cells.
func TestScreenRelativeCursor(t *testing.T) {
	s := NewScreen(6, 3)
	s.Write([]byte("ab\x1b[1D\x1b[1BX\x1b[1A\x1b[1CY"))
	if got := s.Lines(); got[0] != "ab Y" || got[1] != " X" {
		t.Fatalf("rows = %#v", got)
	}
}
