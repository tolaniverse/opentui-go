package opentui

import (
	"bytes"
	"strings"
	"testing"
)

// newPair returns a "screen" buffer that is already in sync with a blank
// frame, plus an empty next buffer.
func newPair(w, h int) (current, next *Buffer) {
	current, next = NewBuffer(w, h), NewBuffer(w, h)
	return current, next
}

func TestEncodeNothingChanged(t *testing.T) {
	cur, next := newPair(4, 2)
	var out bytes.Buffer
	st := encodeFrame(&out, cur, next, FrameOptions{Sync: true})
	if st.Started || out.Len() != 0 {
		t.Fatalf("identical frames must write nothing, wrote %q", out.String())
	}
}

func TestEncodeSingleRun(t *testing.T) {
	cur, next := newPair(5, 1)
	red := RGB(255, 0, 0)
	next.DrawText("hi", 1, 0, red, Transparent, AttrNone)

	var out bytes.Buffer
	st := encodeFrame(&out, cur, next, FrameOptions{})
	want := "\x1b[?25l" + // begin frame: hide cursor
		"\x1b[1;2H" + // move to column 2, row 1 (1-based)
		"\x1b[38;2;255;0;0m" + "\x1b[49m" + // fg red, bg terminal default
		"hi" + // one run, one move
		"\x1b[0m" + // unchanged cell at x=3 ends the run (upstream does this too)
		"\x1b[0m" // end of frame
	if got := out.String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if st.CellsUpdated != 2 {
		t.Errorf("CellsUpdated = %d; want 2", st.CellsUpdated)
	}

	// The current buffer now matches, so a second identical frame is free.
	out.Reset()
	if st := encodeFrame(&out, cur, next, FrameOptions{}); st.Started {
		t.Errorf("second frame wrote %q", out.String())
	}
}

func TestEncodeStyleChangeStartsNewRun(t *testing.T) {
	cur, next := newPair(2, 1)
	next.Set(0, 0, Cell{Char: 'a', FG: RGB(1, 1, 1), BG: Transparent, Attr: AttrBold})
	next.Set(1, 0, Cell{Char: 'b', FG: RGB(2, 2, 2), BG: Transparent})

	var out bytes.Buffer
	encodeFrame(&out, cur, next, FrameOptions{})
	got := out.String()
	if strings.Count(got, "H") != 2 {
		t.Errorf("expected two cursor moves (two runs), got %q", got)
	}
	if !strings.Contains(got, "\x1b[1ma\x1b[0m") {
		t.Errorf("bold run must be closed with reset, got %q", got)
	}
}

func TestEncodeSync(t *testing.T) {
	cur, next := newPair(1, 1)
	next.DrawText("x", 0, 0, White, Transparent, AttrNone)
	var out bytes.Buffer
	encodeFrame(&out, cur, next, FrameOptions{Sync: true})
	got := out.String()
	if !strings.HasPrefix(got, "\x1b[?2026h") || !strings.HasSuffix(got, "\x1b[?2026l") {
		t.Errorf("sync markers missing: %q", got)
	}
}

func TestClearCharForcesRepaint(t *testing.T) {
	cur, next := newPair(3, 1)
	cur.fill(clearChar, RGB(0, 0, 0)) // "screen contents unknown"
	var out bytes.Buffer
	st := encodeFrame(&out, cur, next, FrameOptions{})
	if st.CellsUpdated != 3 {
		t.Errorf("CellsUpdated = %d; want 3 (every cell)", st.CellsUpdated)
	}
	if strings.ContainsRune(out.String(), clearChar) {
		t.Error("clearChar must never reach the terminal")
	}
}

func TestEncodeColorIntents(t *testing.T) {
	cur, next := newPair(1, 1)
	next.Set(0, 0, Cell{Char: 'x', FG: Indexed(9), BG: Default(RGB(0, 0, 0))})
	var out bytes.Buffer
	encodeFrame(&out, cur, next, FrameOptions{})
	if !strings.Contains(out.String(), "\x1b[38;5;9m\x1b[49m") {
		t.Errorf("got %q", out.String())
	}
}
