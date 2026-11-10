package opentui

import "testing"

func testRenderer(root NodeHandle) *Renderer { return &Renderer{root: root.AsNode()} }

func press(name, sequence string) *KeyEvent {
	return &KeyEvent{Name: name, Sequence: sequence, Raw: sequence, Type: KeyPress}
}

func TestFocusTraversalAndValidation(t *testing.T) {
	a := Input(InputOptions{ID: "a"})
	b := Input(InputOptions{ID: "b"})
	c := Input(InputOptions{ID: "c"})
	hidden := NewNode(Style{}, c)
	hidden.Hidden = true
	root := NewNode(Style{}, a, hidden, b)
	r := testRenderer(root)

	if got := r.FocusNext(false); got != a.Node || !a.Focused() {
		t.Fatalf("first focus = %v", got)
	}
	if got := r.FocusNext(false); got != b.Node || a.Focused() || !b.Focused() {
		t.Fatalf("second focus = %v", got)
	}
	if got := r.FocusNext(false); got != a.Node {
		t.Fatalf("wrap focus = %v", got)
	}
	if got := r.FocusNext(true); got != b.Node {
		t.Fatalf("reverse wrap = %v", got)
	}
	if r.Focus(c) {
		t.Fatal("hidden descendant must not focus")
	}

	root.Remove(b)
	r.validateFocus()
	if r.Focused() != nil || b.Focused() {
		t.Fatal("detached focus was not cleared")
	}
}

func TestTabDispatchAndGlobalPrevention(t *testing.T) {
	a, b := Input(InputOptions{}), Input(InputOptions{})
	r := testRenderer(NewNode(Style{}, a, b))
	r.Focus(a)

	tab := press("tab", "\t")
	r.dispatchInput(keyInput{tab})
	if r.Focused() != b.Node || !tab.DefaultPrevented() {
		t.Fatal("Tab did not advance focus")
	}

	shiftTab := press("tab", "\x1b[Z")
	shiftTab.Shift = true
	r.dispatchInput(keyInput{shiftTab})
	if r.Focused() != a.Node {
		t.Fatal("Shift+Tab did not reverse focus")
	}

	r.SetKeyHandler(func(k *KeyEvent) {
		if k.Name == "tab" {
			k.PreventDefault()
		}
	})
	r.dispatchInput(keyInput{press("tab", "\t")})
	if r.Focused() != a.Node {
		t.Fatal("prevented Tab changed focus")
	}
}

func TestGlobalPreventDefaultSuppressesFocusedNode(t *testing.T) {
	n := NewNode(Style{})
	n.Focusable = true
	delivered := false
	n.SetKeyHandler(func(*KeyEvent) { delivered = true })
	r := testRenderer(NewNode(Style{}, n))
	r.Focus(n)
	r.SetKeyHandler(func(k *KeyEvent) { k.PreventDefault() })
	r.dispatchInput(keyInput{press("x", "x")})
	if delivered {
		t.Fatal("globally prevented key reached focused node")
	}
}

func TestInputUserHandlerRunsBeforeEditing(t *testing.T) {
	n := Input(InputOptions{})
	n.SetKeyHandler(func(k *KeyEvent) {
		if k.Name == "x" {
			k.PreventDefault()
		}
	})
	r := testRenderer(NewNode(Style{}, n))
	r.Focus(n)
	r.dispatchInput(keyInput{press("x", "x")})
	r.dispatchInput(keyInput{press("y", "y")})
	if got := n.Value(); got != "y" {
		t.Fatalf("value = %q", got)
	}
}

func TestFocusChangeDoesNotReuseInFlightKey(t *testing.T) {
	a, b := Input(InputOptions{}), Input(InputOptions{})
	r := testRenderer(NewNode(Style{}, a, b))
	a.SetKeyHandler(func(*KeyEvent) { r.Focus(b) })
	r.Focus(a)
	r.dispatchInput(keyInput{press("x", "x")})
	if a.Value() != "x" || b.Value() != "" || r.Focused() != b.Node {
		t.Fatalf("a=%q b=%q focused=%p", a.Value(), b.Value(), r.Focused())
	}
}

func TestFocusCallbacks(t *testing.T) {
	a, b := Input(InputOptions{}), Input(InputOptions{})
	focused, blurred := 0, 0
	a.SetFocusHandler(func() { focused++ })
	a.SetBlurHandler(func() { blurred++ })
	r := testRenderer(NewNode(Style{}, a, b))
	r.Focus(a)
	r.Focus(b)
	if focused != 1 || blurred != 1 {
		t.Fatalf("focus=%d blur=%d", focused, blurred)
	}
}

func TestInputBlurHandlerPreservesChangeCommit(t *testing.T) {
	changes, blurs := 0, 0
	n := Input(InputOptions{OnChange: func(string) { changes++ }})
	n.SetBlurHandler(func() { blurs++ })
	r := testRenderer(NewNode(Style{}, n))
	r.Focus(n)
	n.InsertText("x")
	r.Blur()
	if changes != 1 || blurs != 1 {
		t.Fatalf("changes=%d blurs=%d", changes, blurs)
	}
}

func TestCtrlCDefaultCanBePrevented(t *testing.T) {
	r := testRenderer(NewNode(Style{}))
	k := press("c", "\x03")
	k.Ctrl = true
	if !r.dispatchInput(keyInput{k}) {
		t.Fatal("Ctrl+C should request exit")
	}
	r.SetKeyHandler(func(k *KeyEvent) { k.PreventDefault() })
	k = press("c", "\x03")
	k.Ctrl = true
	if r.dispatchInput(keyInput{k}) {
		t.Fatal("prevented Ctrl+C requested exit")
	}
}

func TestInputEditingAndCallbacks(t *testing.T) {
	var inputs, changes, submits []string
	n := Input(InputOptions{
		Value: "cat", MaxLength: 5, MinLength: 2,
		OnInput:  func(v string) { inputs = append(inputs, v) },
		OnChange: func(v string) { changes = append(changes, v) },
		OnSubmit: func(v string) { submits = append(submits, v) },
	})
	r := testRenderer(NewNode(Style{}, n))
	r.Focus(n)

	n.keyHandler(press("left", "\x1b[D"))
	n.keyHandler(press("x", "x"))
	if got := n.Value(); got != "caxt" || n.Cursor() != 3 {
		t.Fatalf("insert: %q cursor %d", got, n.Cursor())
	}
	n.keyHandler(press("backspace", "\x7f"))
	if got := n.Value(); got != "cat" || n.Cursor() != 2 {
		t.Fatalf("backspace: %q cursor %d", got, n.Cursor())
	}
	n.keyHandler(press("delete", "\x1b[3~"))
	if got := n.Value(); got != "ca" {
		t.Fatalf("delete: %q", got)
	}
	n.InsertText("fé\nZZ") // newline stripped; MaxLength truncates by clusters
	if got := n.Value(); got != "caféZ" {
		t.Fatalf("paste/max: %q", got)
	}
	if !n.Submit() || len(submits) != 1 || submits[0] != "caféZ" {
		t.Fatalf("submit: %v", submits)
	}
	r.Blur()
	if len(changes) != 1 || changes[0] != "caféZ" {
		t.Fatalf("changes: %v", changes)
	}
	if len(inputs) < 4 {
		t.Fatalf("input callbacks: %v", inputs)
	}
}

func TestInputMinLengthAndChangeOnBlur(t *testing.T) {
	changes, submits := 0, 0
	n := Input(InputOptions{MinLength: 3, OnChange: func(string) { changes++ }, OnSubmit: func(string) { submits++ }})
	r := testRenderer(NewNode(Style{}, n))
	r.Focus(n)
	n.InsertText("ab")
	if n.Submit() {
		t.Fatal("short value submitted")
	}
	r.Blur()
	if changes != 1 || submits != 0 {
		t.Fatalf("change=%d submit=%d", changes, submits)
	}
}

func TestInputPaintPlaceholderAndCursor(t *testing.T) {
	n := Input(InputOptions{Placeholder: "name", Style: Style{Width: Cells(6)}})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	b := draw(root, 6, 1)
	if got := screen(b); got != "name  \n" {
		t.Fatalf("placeholder: %q", got)
	}

	r := testRenderer(root)
	r.Focus(n)
	n.SetValue("abcdef")
	n.SetCursor(6)
	b = draw(root, 6, 1)
	if got := screen(b); got != "bcdef \n" {
		t.Fatalf("scrolled value: %q", got)
	}
	cell, _ := b.Get(5, 0)
	if cell.Attr&AttrInverse == 0 {
		t.Fatal("focused cursor is not visible")
	}
}
