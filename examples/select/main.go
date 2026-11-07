//go:build unix

// select shows a Select list: Up/Down or K/J move, Shift+Up/Down jump by
// five, Enter chooses, a click selects and a click on the selected item
// chooses, and the wheel moves the selection. Typing in the filter field
// above narrows the list. Tab switches between the two.
//
//	go run ./examples/select
package main

import (
	"context"
	"fmt"
	"os"

	tui "github.com/tolaniverse/opentui-go"
)

var languages = []tui.SelectOption{
	{Name: "Go", Description: "Simple, fast, concurrent"},
	{Name: "Rust", Description: "Memory safety without a GC"},
	{Name: "Zig", Description: "What upstream OpenTUI's core is written in"},
	{Name: "TypeScript", Description: "Upstream OpenTUI's API"},
	{Name: "Python", Description: "Batteries included"},
	{Name: "C", Description: "Close to the metal"},
	{Name: "Haskell", Description: "Pure and lazy"},
	{Name: "OCaml", Description: "Practical functional programming"},
	{Name: "Elixir", Description: "Fault-tolerant, on the BEAM"},
	{Name: "Kotlin", Description: "A modern JVM language"},
	{Name: "Swift", Description: "Apple's systems language"},
	{Name: "Lua", Description: "Small and embeddable"},
}

func main() {
	bg := tui.MustHex("#1e1e2e")
	field := tui.MustHex("#11111b")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · select"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "select:", err)
		os.Exit(1)
	}

	status := tui.Text("Enter chooses · Tab switches to the filter", tui.TextOptions{FG: green})
	list := tui.Select(tui.SelectOptions{
		Options: languages, ShowScrollIndicator: true,
		Style:     tui.Style{FlexGrow: 1},
		FocusedBG: field, BG: field,
		OnChange: func(_ int, o tui.SelectOption) { status.SetText("Highlighted: " + o.Name) },
		OnSelect: func(_ int, o tui.SelectOption) { status.SetText("Chosen: " + o.Name) },
	})
	filter := tui.Input(tui.InputOptions{
		Placeholder: "Type to filter…", FG: tui.White, BG: field, FocusedBG: field,
		OnInput: func(q string) {
			list.SetFilter(q)
			status.SetText(fmt.Sprintf("%d match(es)", list.VisibleOptions()))
		},
	})

	box := func(title string, child *tui.Node, grow float64) *tui.Node {
		style := tui.Style{FlexGrow: grow, Padding: tui.XY(1, 0)}
		if grow > 0 {
			// Grow into leftover space instead of starting from the list's
			// full natural height, which would squeeze the status line out.
			style.FlexBasis = tui.Cells(0)
		}
		return tui.Box(tui.BoxOptions{
			Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent, Background: field,
			Title: title, TitleColor: accent, Style: style,
		}, child)
	}
	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(box(" Filter ", filter, 0), box(" Languages ", list, 1), status,
		tui.Text("Up/Down or K/J move · Shift jumps · Enter or click chooses · Ctrl+C quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim}))
	r.Focus(list)

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "select:", err)
		os.Exit(1)
	}
}
