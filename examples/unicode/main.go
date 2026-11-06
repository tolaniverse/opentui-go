//go:build unix

// unicode shows terminal-cell layout for wide characters, combining marks,
// and emoji grapheme clusters. Ctrl+C restores the terminal.
//
//	go run ./examples/unicode
package main

import (
	"context"
	"fmt"
	"os"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	r, err := tui.New(tui.Options{Title: "opentui-go · unicode"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "unicode:", err)
		os.Exit(1)
	}
	r.Root().Style = tui.Style{AlignItems: tui.AlignStart}
	r.Root().Add(tui.Text("A界B\ne\u0301X\n👩🏽‍💻!", tui.TextOptions{}))
	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "unicode:", err)
		os.Exit(1)
	}
}
