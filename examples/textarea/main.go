// textarea shows a multiline editor: Enter for new lines, arrows across
// lines, Ctrl+A/E line start/end, Home/End buffer start/end, multi-line
// paste, and Alt+Enter to submit. With the mouse: click to place the
// cursor, drag or double-click to select (copied with OSC 52), Shift+arrows
// to extend, and the wheel to scroll. Typing replaces the selection.
//
//	go run ./examples/textarea
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	tui "github.com/tolaniverse/opentui-go"
)

func main() {
	bg := tui.MustHex("#1e1e2e")
	field := tui.MustHex("#11111b")
	accent := tui.MustHex("#89b4fa")
	green := tui.MustHex("#a6e3a1")
	muted := tui.MustHex("#7f849c")

	r, err := tui.New(tui.Options{Background: bg, Title: "opentui-go · textarea"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "textarea:", err)
		os.Exit(1)
	}

	submitted, copied := "", ""
	editor := tui.Textarea(tui.TextareaOptions{
		ID: "notes", Placeholder: "Write some notes…",
		Style: tui.Style{FlexGrow: 1},
		FG:    tui.White, BG: field, FocusedFG: tui.White, FocusedBG: field,
		OnSubmit: func(value string) {
			submitted = fmt.Sprintf("Submitted %d line(s)", strings.Count(value, "\n")+1)
		},
		OnSelect: func(text string) {
			if r.CopyToClipboard(text, tui.ClipboardSystem) == nil {
				copied = fmt.Sprintf("Copied %d chars", utf8.RuneCountInString(text))
			}
		},
	})
	box := tui.Box(tui.BoxOptions{
		Border: tui.BorderAll, BorderStyle: tui.BorderRounded, BorderColor: accent,
		Title: " Notes ", TitleColor: accent, Background: field,
		Style: tui.Style{FlexGrow: 1, Padding: tui.XY(1, 0)},
	}, editor)
	statusRow := tui.NewNode(tui.Style{Height: tui.Cells(1)})
	footer := tui.Text("Enter newline  •  Alt+Enter submit  •  drag to select + copy  •  wheel scrolls  •  Ctrl+C quit",
		tui.TextOptions{FG: muted, Attr: tui.AttrDim, Wrap: tui.WrapWord})

	r.Root().Style = tui.Style{Gap: 1}
	r.Root().Add(box, statusRow, footer)
	r.Focus(editor)

	// The draw callback runs after the tree is painted, so it always sees
	// the cursor after the latest edit.
	draw := func(b *tui.Buffer) {
		line, col := editor.CursorPosition()
		status := fmt.Sprintf("Ln %d, Col %d · %d line(s)", line+1, col+1, editor.LineCount())
		if submitted != "" {
			status += " · " + submitted
		}
		if copied != "" {
			status += " · " + copied
		}
		rect := statusRow.Rect()
		b.DrawText(status, rect.X, rect.Y, green, tui.Transparent, tui.AttrNone)
	}
	if err := r.Run(context.Background(), draw); err != nil {
		fmt.Fprintln(os.Stderr, "textarea:", err)
		os.Exit(1)
	}
}
