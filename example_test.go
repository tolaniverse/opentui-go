package opentui_test

import (
	"fmt"

	tui "github.com/tolaniverse/opentui-go"
)

func ExampleText() {
	b := tui.NewBuffer(10, 1)
	tui.Draw(tui.Text("Hello", tui.TextOptions{}), b)
	for x := range 5 {
		cell, _ := b.Get(x, 0)
		fmt.Printf("%c", cell.Char)
	}
	fmt.Println()
	// Output: Hello
}

func ExampleBox() {
	b := tui.NewBuffer(14, 3)
	box := tui.Box(tui.BoxOptions{Style: tui.Style{Width: tui.Cells(14), Height: tui.Cells(3)}, Border: tui.BorderAll},
		tui.Text("Hello", tui.TextOptions{}))
	tui.Draw(box, b)
	cell, _ := b.Get(1, 1)
	fmt.Printf("%c\n", cell.Char)
	// Output: H
}

func ExampleInput() {
	input := tui.Input(tui.InputOptions{ID: "name"})
	root := tui.NewNode(tui.Style{}, input)
	input.SetValue("Ada")
	fmt.Println(tui.AsInput(root.Find("name")).Value())
	// Output: Ada
}
