// tasks is a small, persistent task app built only with opentui-go. It is a
// dogfooding example: the task model, storage and UI are deliberately local.
//
//	go run ./examples/tasks [-data /path/to/tasks.json]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	tui "github.com/tolaniverse/opentui-go"
)

type app struct {
	r      *tui.Renderer
	store  *store
	list   *tui.Node
	filter *tui.Node
	editor *tui.Node
	status *tui.Node
	editID int // 0 means add a new task
}

func main() {
	dataPath := flag.String("data", os.Getenv("OPENTUI_TASKS_FILE"), "task data file (default: user config directory)")
	flag.Parse()
	if *dataPath == "" {
		var err error
		*dataPath, err = defaultDataPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "tasks:", err)
			os.Exit(1)
		}
	}
	s, err := openStore(*dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
	r, err := tui.New(tui.Options{Background: tui.MustHex("#1e1e2e"), Title: "opentui-go · tasks"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
	a := newApp(r, s)
	a.build()
	if err := r.Run(context.Background(), nil); err != nil {
		fmt.Fprintln(os.Stderr, "tasks:", err)
		os.Exit(1)
	}
}

func newApp(r *tui.Renderer, s *store) *app { return &app{r: r, store: s} }

func (a *app) build() {
	field := tui.MustHex("#11111b")
	accent := tui.MustHex("#89b4fa")
	muted := tui.MustHex("#7f849c")
	green := tui.MustHex("#a6e3a1")
	a.status = tui.Text("", tui.TextOptions{FG: green, Wrap: tui.WrapWord})
	a.list = tui.Select(tui.SelectOptions{
		Style: tui.Style{FlexGrow: 1, FlexBasis: tui.Cells(0)},
		BG:    field, FocusedBG: field, ShowScrollIndicator: true,
		HideDescription: true,
		OnSelect:        func(_ int, o tui.SelectOption) { a.toggle(o.Value.(int)) },
	})
	a.filter = tui.Input(tui.InputOptions{
		Placeholder: "Type to filter; Enter returns to list", MaxLength: 100,
		Style: tui.Style{Width: tui.Pct(100)}, FG: tui.White, BG: field, FocusedBG: field,
		OnInput:  func(value string) { a.list.SetFilter(value); a.showCounts() },
		OnSubmit: func(string) { a.r.Focus(a.list) },
	})
	a.editor = tui.Input(tui.InputOptions{
		Placeholder: "a: add · e: edit · Enter: save · Esc: cancel", MaxLength: 200,
		Style: tui.Style{Width: tui.Pct(100)}, FG: tui.White, BG: field, FocusedBG: field,
		OnSubmit: a.saveEditor,
	})
	box := func(title string, child *tui.Node, grow bool) *tui.Node {
		style := tui.Style{Padding: tui.XY(1, 0)}
		if grow {
			style.FlexGrow, style.FlexBasis = 1, tui.Cells(0)
		}
		return tui.Box(tui.BoxOptions{
			Border: tui.BorderAll, BorderStyle: tui.BorderRounded,
			BorderColor: accent, Background: field, Title: title, TitleColor: accent,
			Style: style,
		}, child)
	}
	a.r.Root().Style = tui.Style{Gap: 1}
	a.r.Root().Add(
		box(" Tasks ", a.list, true),
		box(" Filter / ", a.filter, false),
		box(" Task title ", a.editor, false),
		a.status,
		tui.Text("↑↓ move · Enter toggles · a add · e edit · / filter · Tab focus · Ctrl+C quit",
			tui.TextOptions{FG: muted, Attr: tui.AttrDim, Wrap: tui.WrapWord}),
	)
	a.r.SetKeyHandler(a.handleKey)
	a.refresh(0)
	a.r.Focus(a.list)
}

func (a *app) showCounts() {
	done := 0
	for _, t := range a.store.data.Tasks {
		if t.Done {
			done++
		}
	}
	a.status.SetText(fmt.Sprintf("%d tasks · %d done · %d visible", len(a.store.data.Tasks), done, a.list.VisibleOptions()))
}

func (a *app) refresh(preferID int) {
	if preferID == 0 {
		if option, ok := a.list.SelectedOption(); ok {
			preferID = option.Value.(int)
		}
	}
	options := make([]tui.SelectOption, 0, len(a.store.data.Tasks))
	for _, t := range a.store.data.Tasks {
		mark := "[ ]"
		if t.Done {
			mark = "[x]"
		}
		options = append(options, tui.SelectOption{Name: mark + " " + t.Title, Value: t.ID})
	}
	a.list.SetSelectOptions(options)
	a.list.SetFilter(a.filter.Value())
	for i, t := range a.store.data.Tasks {
		if t.ID == preferID {
			a.list.SetSelectedIndex(i)
			break
		}
	}
	a.showCounts()
}

func (a *app) toggle(id int) {
	if err := a.store.toggle(id); err != nil {
		a.status.SetText("Save failed: " + err.Error())
		return
	}
	a.refresh(id)
}

func (a *app) saveEditor(value string) {
	if strings.TrimSpace(value) == "" {
		a.status.SetText("Enter a task title before saving")
		return
	}
	id := a.editID
	var err error
	if id == 0 {
		id, err = a.store.add(value)
	} else {
		err = a.store.edit(id, value)
	}
	if err != nil {
		a.status.SetText("Save failed: " + err.Error())
		return
	}
	a.editID = 0
	a.editor.SetValue("")
	a.filter.SetValue("") // reveal the saved item, even if it did not match
	a.refresh(id)
	a.r.Focus(a.list)
}

func (a *app) handleKey(key *tui.KeyEvent) {
	if key.Type == tui.KeyRelease || key.Ctrl || key.Alt || key.Meta || key.Super {
		return
	}
	if key.Name == "escape" && a.r.Focused() != a.list {
		if a.r.Focused() == a.editor {
			a.editor.SetValue("")
			a.editID = 0
		}
		a.r.Focus(a.list)
		key.PreventDefault()
		return
	}
	if a.r.Focused() != a.list {
		return // printable keys belong to the focused Input
	}
	switch key.Name {
	case "/":
		a.r.Focus(a.filter)
	case "a":
		a.editID = 0
		a.editor.SetValue("")
		a.r.Focus(a.editor)
	case "e":
		option, ok := a.list.SelectedOption()
		if !ok {
			return
		}
		a.editID = option.Value.(int)
		for _, t := range a.store.data.Tasks {
			if t.ID == a.editID {
				a.editor.SetValue(t.Title)
				break
			}
		}
		a.r.Focus(a.editor)
	default:
		return
	}
	key.PreventDefault()
}
