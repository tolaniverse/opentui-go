# opentui-go

A pure-Go port of [OpenTUI](https://github.com/anomalyco/opentui), a library for building terminal user interfaces.

Upstream OpenTUI pairs a Zig engine with a TypeScript API. This port reimplements both layers in Go, with no cgo and no Zig.

> **Status: v0.2.0, early.** It includes renderable trees, Yoga-compatible flex layout, boxes, borders, clipping, styled text, diff rendering, resize handling and clean shutdown.
> Unix only (macOS, Linux, BSD).

Ported from upstream commit [`e4ad586`](https://github.com/anomalyco/opentui/tree/e4ad5867eae1051a2c1877b08d88bbcaf12b7209).

## Try it

```bash
go run ./examples/hello
```

Resize the window to watch the two-panel tree reflow. Press Ctrl+C to quit; the terminal is restored.

## Use it

```go
r, err := opentui.New(opentui.Options{Background: opentui.MustHex("#1e1e2e")})
if err != nil { log.Fatal(err) }

sidebar := opentui.Box(opentui.BoxOptions{
	Border: opentui.BorderAll,
	Title: " Menu ",
	Style: opentui.Style{Width: opentui.Pct(30), MinWidth: opentui.Cells(20)},
}, opentui.Text("Home\nSettings", opentui.TextOptions{}))

content := opentui.Box(opentui.BoxOptions{
	Border: opentui.BorderAll,
	Style: opentui.Style{FlexGrow: 1, Padding: opentui.All(1)},
}, opentui.Text("Hello, OpenTUI-Go!", opentui.TextOptions{}))

r.Root().Style.Direction = opentui.Row
r.Root().Add(sidebar, content)
err = r.Run(context.Background(), nil)
```

`Run` handles Ctrl+C, SIGINT, SIGTERM, SIGHUP and resizes. It restores the terminal on every exit path, including a panic in your draw function.

## Roadmap

| Release | Scope |
|---|---|
| **v0.1.0** | Terminal lifecycle, cells, colors, alpha blending, diff renderer |
| **v0.2.0** | Renderable tree, boxes, borders, clipping, Yoga-compatible basic layout |
| v0.3.0 | Keyboard decoding, focus, text input |
| v0.4.0 | Scrolling, selection, mouse |
| v0.5.0 | Demo app, compatibility notes, performance pass |

## Layout

`Style` supports row/column directions (including reverse), grow and shrink, basis, fixed and percentage dimensions, min/max constraints, gap, padding, margins, justify/align, and absolute positioning. Layout is a pure-Go port of the supported Yoga 3.2.1 algorithm and is checked against 410 generated real-Yoga cases. It also carries OpenTUI's flex-shrink residue fix.

## Known limitations (v0.2.0)

- Each rune occupies one cell; wide characters (CJK, emoji) and grapheme clusters are not handled yet.
- Truecolor is assumed; there is no terminal capability detection yet.
- No hyperlinks, images or cursor control.
- Input handling covers Ctrl+C only.
- No flex wrapping, align-content, baseline alignment, aspect ratio, percentage padding/margins, or auto margins yet.
- `Text` does not wrap and supports plain text only; styled spans arrive in a later release.

## License

MIT. See [LICENSE](LICENSE). Includes code ported from OpenTUI (MIT, © 2025 opentui).
