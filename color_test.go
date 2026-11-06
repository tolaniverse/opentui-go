package opentui

import "testing"

func TestHex(t *testing.T) {
	cases := map[string]RGBA{
		"#ff8000":   RGB(0xff, 0x80, 0x00),
		"ff8000":    RGB(0xff, 0x80, 0x00),
		"#f80":      RGB(0xff, 0x88, 0x00),
		"#f808":     RGBAOf(0xff, 0x88, 0x00, 0x88),
		"#11223344": RGBAOf(0x11, 0x22, 0x33, 0x44),
	}
	for in, want := range cases {
		got, err := Hex(in)
		if err != nil || got != want {
			t.Errorf("Hex(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "#12", "#12345", "#gggggg"} {
		if _, err := Hex(bad); err == nil {
			t.Errorf("Hex(%q) should fail", bad)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, s := range []string{"#0a0b0c", "#0a0b0c80"} {
		if got := MustHex(s).String(); got != s {
			t.Errorf("String() = %q; want %q", got, s)
		}
	}
}

func TestAnsi256(t *testing.T) {
	cases := map[uint8]RGBA{
		1:   RGB(0x80, 0, 0),    // basic 16
		16:  RGB(0, 0, 0),       // cube start
		196: RGB(255, 0, 0),     // cube: (180/36)=5 -> 255
		231: RGB(255, 255, 255), // cube end
		232: RGB(8, 8, 8),       // grayscale start
		255: RGB(238, 238, 238), // grayscale end
	}
	for i, want := range cases {
		if got := Ansi256(i); got != want {
			t.Errorf("Ansi256(%d) = %v; want %v", i, got, want)
		}
	}
	if c := Indexed(196); c.Intent != IntentIndexed || c.Slot != 196 || c.R != 255 {
		t.Errorf("Indexed(196) = %+v", c)
	}
}

func TestBlend(t *testing.T) {
	white, black := RGB(255, 255, 255), RGB(0, 0, 0)

	if got := blend(RGBAOf(9, 9, 9, 0), black, nil); got != black {
		t.Errorf("transparent src must keep dst, got %v", got)
	}
	if got := blend(white, black, nil); got != white {
		t.Errorf("opaque src must win, got %v", got)
	}
	// 50% white over black -> mid gray: (255*128 + 0*127 + 127)/255 = 128
	if got := blend(RGBAOf(255, 255, 255, 128), black, nil); got != RGB(128, 128, 128) {
		t.Errorf("half white over black = %v", got)
	}
	// Transparent dst falls back to the backdrop.
	bd := RGB(0, 0, 255)
	if got := blend(RGBAOf(255, 0, 0, 128), Transparent, &bd); got != RGB(128, 0, 127) {
		t.Errorf("blend over backdrop = %v", got)
	}
	// Indexed colors become literal RGB once blended.
	if got := blend(RGBAOf(0, 0, 0, 128), Indexed(15), nil); got.Intent != IntentRGB {
		t.Errorf("blended color intent = %v; want rgb", got.Intent)
	}
}
