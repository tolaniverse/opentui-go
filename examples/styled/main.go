//go:build unix

// styled shows rich text: bold, italic, underlined and colored spans in one
// word-wrapped Text, plus an OSC 8 hyperlink. Links are sent only to
// terminals known to support them (FORCE_HYPERLINK=1 forces them on).
//
//	go run ./examples/styled
package main

import (
	"context"
	"fmt"
	"os"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	bg := tui.MustHex("#1e1e2e")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	red := tui.MustHex("#f38ba8")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · styled"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "styled:", err)
		os.Exit(1)
	}

	body := tui.Styled(tui.TextOptions{Wrap: tui.WrapWord},
		tui.S("Styled text").Bold().Fg(accent),
		tui.S(" mixes "),
		tui.S("bold").Bold(), tui.S(", "),
		tui.S("italic").Italic(), tui.S(", "),
		tui.S("underlined").Underline(), tui.S(" and "),
		tui.S("colored").Fg(green), tui.S(" spans in one wrapped paragraph.\n\n"),
		tui.S("Errors").Bold().Fg(red), tui.S(" stand out. "),
		tui.S("Links: "), tui.Link("https://github.com/tolaniverse/opentui-go", "opentui-go on GitHub").Underline().Fg(accent),
		tui.S("."),
	)
	links := "off (FORCE_HYPERLINK=1 to force)"
	if r.Hyperlinks() {
		links = "on"
	}
	status := tui.Text("Hyperlinks "+links+" · colors "+r.ColorMode().String(), tui.TextOptions{FG: muted})

	box := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent,
		Title: " Rich text ", TitleColor: accent,
		Style: tui.Style{FlexGrow: 1, Padding: tui.XY(1, 0)},
	}, body)
	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(box, status, tui.Text("Ctrl+C quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim}))

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "styled:", err)
		os.Exit(1)
	}
}
