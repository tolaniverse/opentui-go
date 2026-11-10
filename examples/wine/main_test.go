package main

import (
	"math"
	"testing"

	tui "github.com/tolaniverse/opentui-go"
)

func TestCheersAndSpill(t *testing.T) {
	before := newScene(clinkTime-0.01, 0)
	if before.flash != 0 || len(before.falling) != 0 {
		t.Fatal("clink effects started before contact")
	}
	at := newScene(clinkTime, 0)
	if math.Abs(at.glass[0].cx+at.glass[1].cx) > 1e-9 ||
		math.Abs(at.glass[1].cx-at.glass[0].cx-2*contactX) > 1e-9 {
		t.Fatal("bowls do not meet at contact")
	}
	after := newScene(clinkTime+0.05, 0)
	if after.flash <= 0 || len(after.falling) == 0 {
		t.Fatal("clink did not produce a glint and falling wine")
	}
	if newScene(16, 0).flash >= after.flash {
		t.Fatal("glint did not decay")
	}
}

func BenchmarkWineFrame(b *testing.B) {
	const width, height = 160, 96 // 160×48 terminal cells
	pixels := make([]tui.RGBA, width*height)
	for _, tc := range []struct {
		name string
		t    float64
	}{
		{"pour", 4},
		{"spill", 11.5},
		{"puddle", 16},
	} {
		b.Run(tc.name, func(b *testing.B) {
			s := newScene(tc.t, 0)
			b.ResetTimer()
			for range b.N {
				render(s, pixels, width, height, 1)
			}
		})
	}
}
