// tabs shows a TabSelect: Left/Right or [/] switch tabs, Enter chooses, a
// click selects a tab and a click on the selected tab chooses it, and the
// wheel moves between tabs. The panel below follows the selected tab.
//
//	go run ./examples/tabs
package main

import (
	"context"
	"fmt"
	"os"

	tui "github.com/tolaniverse/opentui-go"
)

var tabs = []tui.SelectOption{
	{Name: "Overview", Description: "What this example shows", Value: "A TabSelect is a focusable row of fixed-width tabs, ported from upstream OpenTUI's TabSelectRenderable."},
	{Name: "Keys", Description: "Keyboard navigation", Value: "Left or [ moves left, Right or ] moves right, Enter chooses the selected tab."},
	{Name: "Mouse", Description: "Pointer and wheel", Value: "Click a tab to select it; click the selected tab to choose it. The wheel moves between tabs."},
	{Name: "Overflow", Description: "Too many tabs for the width", Value: "When the tabs do not fit, the selection stays centered and ‹ › arrows show that more tabs are hidden."},
	{Name: "Truncation", Description: "Names longer than a tab", Value: "Names wider than a tab end in an ellipsis, as upstream's drawTextFit."},
	{Name: "A very long tab name", Description: "This one is truncated", Value: "Its name does not fit in 16 cells."},
}

func main() {
	bg := tui.MustHex("#1e1e2e")
	field := tui.MustHex("#11111b")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · tabs"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "tabs:", err)
		os.Exit(1)
	}

	panel := tui.Text(tabs[0].Value.(string), tui.TextOptions{FG: tui.White, Wrap: tui.WrapWord})
	status := tui.Text("Enter chooses · Ctrl+C quits", tui.TextOptions{FG: green})
	bar := tui.TabSelect(tui.TabSelectOptions{
		Options: tabs, TabWidth: 16, BG: field,
		OnChange: func(_ int, o tui.SelectOption) {
			panel.SetText(o.Value.(string))
			status.SetText("Showing: " + o.Name)
		},
		OnSelect: func(_ int, o tui.SelectOption) { status.SetText("Chosen: " + o.Name) },
	})

	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(
		bar,
		tui.Box(tui.BoxOptions{
			Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent, Background: field,
			Title: " Panel ", TitleColor: accent, Style: tui.Style{FlexGrow: 1, Padding: tui.XY(1, 0)},
		}, panel),
		status,
		tui.Text("Left/Right or [/] switch · Enter or click chooses · wheel moves · Ctrl+C quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim, Wrap: tui.WrapWord}),
	)
	r.Focus(bar)

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "tabs:", err)
		os.Exit(1)
	}
}
