// wrap shows soft wrapping: a word-wrapped article that reflows when the
// terminal is resized, beside a word-wrapping editor where Up/Down move by
// visual row. Drag across the article to select; a soft wrap adds no "\n".
//
//	go run ./examples/wrap
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	tui "github.com/tolaniverse/opentui-go"
)

const article = "Soft wrapping breaks a long line into rows that fit the width of its box. " +
	"Word mode breaks at Unicode line-break opportunities, such as after a space; " +
	"a word longer than the box falls back to breaking between characters. " +
	"Wide characters like 界 and emoji like 👩🏽‍💻 are never split across rows.\n\n" +
	"Resize the terminal and this text reflows. END."

func main() {
	bg := tui.MustHex("#1e1e2e")
	panel := tui.MustHex("#181825")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · wrap"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "wrap:", err)
		os.Exit(1)
	}

	selected := ""
	text := tui.Text(article, tui.TextOptions{FG: tui.White, Wrap: tui.WrapWord})
	left := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent, Background: panel,
		Title: " Article (word wrap) ", TitleColor: accent,
		Style: tui.Style{FlexGrow: 1, FlexBasis: tui.Cells(0), Padding: tui.XY(1, 0)},
	}, text)
	editor := tui.Textarea(tui.TextareaOptions{
		Placeholder: "Type a long line…", Wrap: tui.WrapWord,
		Style: tui.Style{FlexGrow: 1}, FG: tui.White, BG: panel, FocusedBG: panel,
	})
	right := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent, Background: panel,
		Title: " Editor (word wrap) ", TitleColor: accent,
		Style: tui.Style{FlexGrow: 1, FlexBasis: tui.Cells(0), Padding: tui.XY(1, 0)},
	}, editor)
	row := tui.NewNode(tui.Style{Direction: tui.Row, FlexGrow: 1, Gap: 1}, left, right)
	statusRow := tui.NewNode(tui.Style{Height: tui.Cells(1)})
	footer := tui.Text("Drag the article to select  •  Up/Down move by visual row  •  Ctrl+C quit",
		tui.TextOptions{FG: muted, Attr: tui.AttrDim, Wrap: tui.WrapWord})

	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(row, statusRow, footer)
	r.Focus(editor)
	r.SetSelectionHandler(func(s *tui.Selection) {
		if t := s.Text(); t != "" {
			selected = t
		}
	})

	draw := func(b *tui.Buffer) {
		line, col := editor.CursorPosition()
		status := fmt.Sprintf("Rows %d · Ln %d, Col %d", text.Rect().H, line+1, col+1)
		if selected != "" {
			status += fmt.Sprintf(" · Sel %d chars, %d newlines",
				utf8.RuneCountInString(selected), strings.Count(selected, "\n"))
		}
		rect := statusRow.Rect()
		b.DrawText(status, rect.X, rect.Y, green, tui.Transparent, tui.AttrNone)
	}
	if err := r.Run(context.Background(), draw); err != nil {
		fmt.Fprintln(os.Stderr, "wrap:", err)
		os.Exit(1)
	}
}
