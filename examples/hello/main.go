//go:build unix

// hello is the v0.1.0 demo: styled text, a translucent overlay, a live
// clock, and resize handling. Press Ctrl+C to quit.
//
//	go run ./examples/hello
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	r, err := tui.New(tui.Options{
		Background: tui.MustHex("#1e1e2e"),
		Title:      "opentui-go hello",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}

	// Redraw once a second so the clock ticks. RequestRender is safe to
	// call from another goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.RequestRender()
			}
		}
	}()

	accent := tui.MustHex("#89b4fa")
	muted := tui.MustHex("#7f849c")

	err = r.Run(ctx, func(b *tui.Buffer) {
		w, h := b.Width(), b.Height()

		b.DrawText("Hello, OpenTUI-Go!", 2, 1, accent, tui.Transparent, tui.AttrBold)
		b.DrawText("bold", 2, 3, tui.White, tui.Transparent, tui.AttrBold)
		b.DrawText("italic", 8, 3, tui.White, tui.Transparent, tui.AttrItalic)
		b.DrawText("underline", 16, 3, tui.White, tui.Transparent, tui.AttrUnderline)
		b.DrawText("strike", 27, 3, tui.White, tui.Transparent, tui.AttrStrikethrough)
		b.DrawText("palette 196", 2, 4, tui.Indexed(196), tui.Transparent, tui.AttrNone)

		// A translucent panel over text: the text stays visible but dimmed.
		b.DrawText("text under a 50% black overlay", 2, 6, tui.White, tui.Transparent, tui.AttrNone)
		b.FillRect(2, 6, 14, 1, tui.RGBAOf(0, 0, 0, 128))

		b.DrawText(fmt.Sprintf("size %dx%d  ·  %s", w, h, time.Now().Format("15:04:05")),
			2, 8, muted, tui.Transparent, tui.AttrNone)
		// Pin the hint to the bottom, but never above the content (row 10).
		b.DrawText("resize the window · Ctrl+C to quit", 2, max(h-2, 10), muted, tui.Transparent, tui.AttrDim)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}
