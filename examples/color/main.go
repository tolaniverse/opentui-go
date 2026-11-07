//go:build unix

// color shows the detected color mode and a colored cell. Detection uses
// TERM, COLORTERM, and NO_COLOR; Options.Color can force a mode.
//
//	go run ./examples/color
package main

import (
	"context"
	"fmt"
	"os"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	r, err := tui.New(tui.Options{
		Background: tui.RGB(30, 30, 46),
		Title:      "opentui-go · color",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "color:", err)
		os.Exit(1)
	}
	label := tui.Text("mode "+r.ColorMode().String(), tui.TextOptions{
		FG: tui.RGB(250, 10, 10),
		BG: tui.RGB(0, 0, 100),
	})
	r.Root().Add(label)
	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "color:", err)
		os.Exit(1)
	}
}
