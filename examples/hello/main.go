//go:build unix

// hello is the v0.4.0 demo: mouse routing, scrolling, text selection,
// keyboard input, focus, and resize handling.
//
//	go run ./examples/hello
package main

import (
	"context"
	"fmt"
	"os"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	bg := tui.MustHex("#1e1e2e")
	panel := tui.MustHex("#181825")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go v0.4.0"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}

	status := tui.Text("Drag text to select · wheel or arrows scroll", tui.TextOptions{FG: green})
	lines := make([]*tui.Node, 24)
	for i := range lines {
		lines[i] = tui.Text(fmt.Sprintf("Line %02d  Select and scroll this document.", i+1), tui.TextOptions{FG: tui.White})
	}
	scroll := tui.ScrollBox(tui.ScrollBoxOptions{
		BoxOptions: tui.BoxOptions{
			Border: tui.BorderAll, BorderColor: accent, Background: panel,
			Title: " Document ", TitleColor: accent,
			Style: tui.Style{FlexGrow: 1, Padding: tui.All(1)},
		},
		OnScroll: func(_, y int) { status.SetText(fmt.Sprintf("Scrolled to line %d", y+1)) },
	}, lines...)

	header := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent,
		Title: " opentui-go v0.4.0 ", TitleColor: accent,
		Style: tui.Style{Height: tui.Cells(3), Padding: tui.XY(1, 0)},
	}, tui.Text("Pure Go mouse input · scrolling · text selection", tui.TextOptions{FG: tui.White}))
	footer := tui.Text("Click focus  •  wheel/arrows scroll  •  drag selects  •  Ctrl+C quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim})

	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(header, scroll, status, footer)
	r.SetSelectionHandler(func(selection *tui.Selection) {
		status.SetText("Selected: " + selection.Text())
	})
	r.Focus(scroll)

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}
