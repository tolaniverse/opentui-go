// document shows a ScrollBox (wheel, keys, and a draggable scrollbar) and
// text selection with auto-scroll and OSC 52 clipboard copy.
//
//	go run ./examples/document
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	bg := tui.MustHex("#1e1e2e")
	panel := tui.MustHex("#181825")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · document"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "document:", err)
		os.Exit(1)
	}

	status := tui.Text("Drag text to select and copy · wheel, arrows or scrollbar scroll", tui.TextOptions{FG: green})
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
		// y counts the content's top padding row, so line N is at the top when y == N.
		OnScroll: func(_, y int) { status.SetText(fmt.Sprintf("Scrolled to line %d", max(y, 1))) },
	}, lines...)

	header := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent,
		Title: " opentui-go · document ", TitleColor: accent,
		Style: tui.Style{Height: tui.Cells(3), Padding: tui.XY(1, 0)},
	}, tui.Text("Pure Go mouse input · scrolling · text selection", tui.TextOptions{FG: tui.White}))
	footer := tui.Text("Wheel/arrows/scrollbar scroll  •  drag selects + copies  •  Ctrl+C quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim, Wrap: tui.WrapWord})

	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(header, scroll, status, footer)
	r.SetSelectionHandler(func(selection *tui.Selection) {
		text := selection.Text()
		if text == "" {
			return
		}
		lines := strings.Split(text, "\n")
		verb := "Copied"
		if err := r.CopyToClipboard(text, tui.ClipboardSystem); err != nil {
			verb = "Selected"
		}
		status.SetText(fmt.Sprintf("%s %d line(s): %s", verb, len(lines), lines[0]))
	})
	r.Focus(scroll)

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "document:", err)
		os.Exit(1)
	}
}
