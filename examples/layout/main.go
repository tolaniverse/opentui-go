// layout shows a renderable tree, Yoga-compatible flex layout, boxes,
// borders, titles, clipping, live updates from another goroutine, and resize
// handling.
//
//	go run ./examples/layout
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	bg := tui.MustHex("#1e1e2e")
	panel := tui.MustHex("#181825")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · layout"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "layout:", err)
		os.Exit(1)
	}

	clock := tui.Text("", tui.TextOptions{FG: muted})
	header := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent,
		Title: " opentui-go ", TitleColor: accent,
		Style: tui.Style{Height: tui.Cells(3), Padding: tui.XY(1, 0)},
	}, tui.Text("Pure Go · renderable trees · Yoga-compatible layout", tui.TextOptions{FG: tui.White}))

	nav := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderColor: muted, Background: panel,
		Title: " Components ", TitleColor: green,
		Style: tui.Style{Width: tui.Pct(30), MinWidth: tui.Cells(20), Padding: tui.All(1), Overflow: tui.Hidden},
	}, tui.Text("● Node tree\n● Flex layout\n● Box + borders\n● Text leaves\n● Clip stack", tui.TextOptions{FG: green}))

	content := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderColor: accent, Background: panel,
		Title: " Live layout ", TitleColor: accent,
		Style: tui.Style{FlexGrow: 1, Padding: tui.All(1), Overflow: tui.Hidden},
	}, tui.Text("Resize this terminal.\n\nThe panels recompute from the same tree;\nonly changed cells are written.", tui.TextOptions{FG: tui.White}))

	body := tui.NewNode(tui.Style{Direction: tui.Row, FlexGrow: 1, Gap: 1}, nav, content)
	footer := tui.NewNode(tui.Style{Height: tui.Cells(1), Direction: tui.Row, Justify: tui.JustifySpaceBetween},
		tui.Text("Ctrl+C to quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim}), clock)
	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(header, body, footer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if !r.Update(func() { clock.SetText(now.Format("15:04:05")) }) {
					return
				}
			}
		}
	}()

	clock.SetText(time.Now().Format("15:04:05"))
	if err := r.Run(ctx, nil); err != nil {
		fmt.Fprintln(os.Stderr, "layout:", err)
		os.Exit(1)
	}
}
