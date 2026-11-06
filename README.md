# opentui-go

A pure-Go port of [OpenTUI](https://github.com/anomalyco/opentui), a library for building terminal user interfaces.

Upstream OpenTUI pairs a Zig engine with a TypeScript API. This port reimplements both layers in Go, with no cgo and no Zig.

> **Status: v0.3.0, early.** It includes incremental keyboard decoding, focus management, single-line text input, renderable trees, Yoga-compatible flex layout, diff rendering, resize handling, and clean shutdown.
> Unix only (macOS, Linux, BSD).

Ported from upstream commit [`e4ad586`](https://github.com/anomalyco/opentui/tree/e4ad5867eae1051a2c1877b08d88bbcaf12b7209).

## Try it

```bash
go run ./examples/hello
```

Type in the form, paste text, use Tab / Shift+Tab to move focus, Enter to submit, and Escape to clear focus. Resize the window to watch the tree reflow. Press Ctrl+C to quit; the terminal is restored.

## Use it

```go
r, err := opentui.New(opentui.Options{Background: opentui.MustHex("#1e1e2e")})
if err != nil { log.Fatal(err) }

name := opentui.Input(opentui.InputOptions{
	Placeholder: "Ada Lovelace",
	MaxLength: 40,
	OnSubmit: func(value string) { log.Printf("hello, %s", value) },
})
r.Root().Add(
	opentui.Text("Your name", opentui.TextOptions{}),
	name,
)
r.Focus(name)
err = r.Run(context.Background(), nil)
```

`Run` handles decoded keyboard and bracketed-paste input, Ctrl+C, SIGINT, SIGTERM, SIGHUP, and resizes. It restores the terminal on every exit path, including a panic in your draw function.

## Keyboard and focus

- `KeyEvent` reports printable keys, controls, arrows, function keys, modifiers, and press/repeat/release events from traditional terminal sequences and CSI-u/Kitty input.
- `Renderer.SetKeyHandler` receives keys before the focused node. `PreventDefault` suppresses built-in Tab and Ctrl+C behavior; `StopPropagation` stops focused-node delivery.
- `Renderer.Focus`, `Blur`, `Focused`, and `FocusNext` manage focus. Tab and Shift+Tab traverse visible focusable nodes in depth-first order and wrap.
- `Input` supports cursor movement, Home/End, Backspace/Delete, Ctrl+A/E/U/K/W, bracketed paste, horizontal scrolling, placeholders, length limits, and input/change/submit callbacks.

## Roadmap

| Release | Scope |
|---|---|
| **v0.1.0** | Terminal lifecycle, cells, colors, alpha blending, diff renderer |
| **v0.2.0** | Renderable tree, boxes, borders, clipping, Yoga-compatible basic layout |
| **v0.3.0** | Keyboard decoding, focus, text input |
| v0.4.0 | Scrolling, selection, mouse |
| v0.5.0 | Demo app, compatibility notes, performance pass |

## Layout

`Style` supports row/column directions (including reverse), grow and shrink, basis, fixed and percentage dimensions, min/max constraints, gap, padding, margins, justify/align, and absolute positioning. Layout is a pure-Go port of the supported Yoga 3.2.1 algorithm and is checked against 410 generated real-Yoga cases. It also carries OpenTUI's flex-shrink residue fix.

## Known limitations (v0.3.0)

- Each rune occupies one cell; wide characters (CJK, emoji) and grapheme clusters are not handled yet.
- Truecolor is assumed; there is no terminal capability detection yet.
- No hyperlinks, images or cursor control.
- Mouse reports are recognized and suppressed, but mouse interaction arrives in v0.4.0.
- Input is single-line only; selection and multiline editing are not implemented yet.
- No flex wrapping, align-content, baseline alignment, aspect ratio, percentage padding/margins, or auto margins yet.
- `Text` does not wrap and supports plain text only; styled spans arrive in a later release.

## License

MIT. See [LICENSE](LICENSE). Includes code ported from OpenTUI (MIT, © 2025 opentui).
