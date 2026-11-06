//go:build unix

// hello is the v0.3.0 demo: keyboard decoding, focus traversal, bracketed
// paste, single-line inputs, Yoga-compatible layout, and resize handling.
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
	bg := tui.MustHex("#1e1e2e")
	panel := tui.MustHex("#181825")
	field := tui.MustHex("#11111b")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go v0.3.0"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}

	status := tui.Text("Ready — type your name", tui.TextOptions{FG: green})
	var email *tui.Node
	name := tui.Input(tui.InputOptions{
		ID: "name", Placeholder: "Ada Lovelace", MaxLength: 32,
		Style: tui.Style{Width: tui.Pct(100), Height: tui.Cells(1)},
		FG:    tui.White, BG: field, FocusedFG: tui.White, FocusedBG: accent,
		OnSubmit: func(value string) {
			status.SetText("Name saved — enter your email")
			r.Focus(email)
		},
	})
	email = tui.Input(tui.InputOptions{
		ID: "email", Placeholder: "ada@example.com", MinLength: 3, MaxLength: 64,
		Style: tui.Style{Width: tui.Pct(100), Height: tui.Cells(1)},
		FG:    tui.White, BG: field, FocusedFG: tui.White, FocusedBG: accent,
		OnSubmit: func(value string) {
			status.SetText("Submitted: " + name.Value() + " <" + value + ">")
		},
	})
	name.SetFocusHandler(func() { status.SetText("Name — Enter moves to email") })
	email.SetFocusHandler(func() { status.SetText("Email — Enter submits") })

	header := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent,
		Title: " opentui-go v0.3.0 ", TitleColor: accent,
		Style: tui.Style{Height: tui.Cells(3), Padding: tui.XY(1, 0)},
	}, tui.Text("Pure Go keyboard input · focus · editing · bracketed paste", tui.TextOptions{FG: tui.White}))

	nameField := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderColor: muted, Background: field,
		Title: " Name ", TitleColor: accent,
		Style: tui.Style{Height: tui.Cells(3), Padding: tui.XY(1, 0)},
	}, name)
	emailField := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderColor: muted, Background: field,
		Title: " Email ", TitleColor: accent,
		Style: tui.Style{Height: tui.Cells(3), Padding: tui.XY(1, 0)},
	}, email)
	form := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderColor: accent, Background: panel,
		Title: " Interactive form ", TitleColor: accent,
		Style: tui.Style{FlexGrow: 1, Gap: 1, Padding: tui.All(1), Overflow: tui.Hidden},
	}, nameField, emailField, status)

	footer := tui.Text("Tab / Shift+Tab focus  •  paste text  •  Enter submit  •  Esc blur  •  Ctrl+C quit", tui.TextOptions{FG: muted, Attr: tui.AttrDim})
	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(header, form, footer)
	r.SetKeyHandler(func(key *tui.KeyEvent) {
		if key.Name == "escape" {
			r.Blur()
			status.SetText("Focus cleared — press Tab to resume")
			key.PreventDefault()
		}
	})
	r.Focus(name)

	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "hello:", err)
		os.Exit(1)
	}
}
