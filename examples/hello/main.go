// hello is the smallest complete opentui-go program: one bordered box with
// text, centred on screen. Ctrl+C quits and restores the terminal.
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
	r, err := tui.New(tui.Options{Background: tui.MustHex("#1e1e2e"), Title: "opentui-go · hello"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}

	greeting := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: tui.MustHex("#89b4fa"),
		Title: " hello ", TitleAlign: tui.TitleCenter,
		Style: tui.Style{Padding: tui.XY(2, 1)},
	}, tui.Text("Hello, OpenTUI-Go!\n\nPress Ctrl+C to quit.", tui.TextOptions{}))

	root := r.Root()
	root.Style = tui.Style{Justify: tui.JustifyCenter, AlignItems: tui.AlignCenter}
	root.Add(greeting)

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}
