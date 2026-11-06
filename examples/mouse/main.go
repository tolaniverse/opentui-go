//go:build unix

// mouse shows pointer events: hover (over/out), clicks with bubbling, and
// drag-and-drop with capture. Drag the card between the three zones.
//
//	go run ./examples/mouse
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

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · mouse"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mouse:", err)
		os.Exit(1)
	}

	status := tui.Text("Move, click, or drag the card", tui.TextOptions{FG: green})

	// The card is a small box; pressing it starts a captured drag. Its text is
	// NoSelect: a press on selectable text starts a text selection, which
	// would own the drag instead.
	card := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: green,
		Style: tui.Style{Padding: tui.XY(1, 0), AlignSelf: tui.AlignStart},
	}, tui.Text("card", tui.TextOptions{FG: green, NoSelect: true}))
	isCard := func(n *tui.Node) bool {
		for ; n != nil; n = n.Parent() {
			if n == card {
				return true
			}
		}
		return false
	}
	card.SetMouseEventHandler(tui.MouseDown, func(e *tui.MouseEvent) {
		status.SetText("Picked up the card — drag it to another zone")
		e.StopPropagation() // a press on the card is not a click on its zone
	})

	zones := make([]*tui.Node, 3)
	for i, name := range []string{"A", "B", "C"} {
		label := tui.Text(fmt.Sprintf("clicks: %d", 0), tui.TextOptions{FG: tui.White, NoSelect: true})
		clicks := 0
		zone := tui.Box(tui.BoxOptions{
			Border: tui.BorderAll, BorderColor: accent, Background: panel,
			Title: " Zone " + name + " ", TitleColor: accent,
			Style: tui.Style{FlexGrow: 1, Padding: tui.All(1), Gap: 1},
		}, label)
		zone.SetMouseEventHandler(tui.MouseOver, func(*tui.MouseEvent) {
			status.SetText("Hovering zone " + name)
		})
		zone.SetMouseEventHandler(tui.MouseDown, func(*tui.MouseEvent) {
			clicks++
			label.SetText(fmt.Sprintf("clicks: %d", clicks))
			status.SetText(fmt.Sprintf("Clicked zone %s (%d)", name, clicks))
		})
		zone.SetMouseEventHandler(tui.MouseDrop, func(e *tui.MouseEvent) {
			if !isCard(e.Source) {
				return
			}
			zone.Add(card) // Add moves a node that already has a parent
			status.SetText("Dropped the card on zone " + name)
		})
		zones[i] = zone
	}
	zones[0].Add(card)

	header := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent,
		Title: " opentui-go · mouse ", TitleColor: accent,
		Style: tui.Style{Height: tui.Cells(3), Padding: tui.XY(1, 0)},
	}, tui.Text("Hover · click · drag and drop", tui.TextOptions{FG: tui.White}))
	row := tui.NewNode(tui.Style{Direction: tui.Row, FlexGrow: 1, Gap: 1}, zones...)
	footer := tui.Text("Ctrl+C quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim})

	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(header, row, status, footer)

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "mouse:", err)
		os.Exit(1)
	}
}
