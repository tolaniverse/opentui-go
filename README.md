# opentui-go

A pure-Go port of [OpenTUI](https://github.com/anomalyco/opentui), a library for building terminal user interfaces.

Upstream OpenTUI pairs a Zig engine with a TypeScript API. This port reimplements both layers in Go, with no cgo and no Zig.

> **Status: v0.1.0, early.** It includes styled text, colors with alpha blending, diff rendering, resize handling and clean shutdown.
> Unix only (macOS, Linux, BSD).

Ported from upstream commit [`e4ad586`](https://github.com/anomalyco/opentui/tree/e4ad5867eae1051a2c1877b08d88bbcaf12b7209).

## Try it

```bash
go run ./examples/hello
```

Resize the window to see a repaint. Press Ctrl+C to quit; the terminal is restored.

## Use it

```go
r, err := opentui.New(opentui.Options{Background: opentui.MustHex("#1e1e2e")})
if err != nil { log.Fatal(err) }

err = r.Run(context.Background(), func(b *opentui.Buffer) {
	b.DrawText("Hello, OpenTUI-Go!", 2, 1, opentui.MustHex("#89b4fa"), opentui.Transparent, opentui.AttrBold)
})
```

`Run` handles Ctrl+C, SIGINT, SIGTERM, SIGHUP and resizes. It restores the terminal on every exit path, including a panic in your draw function.

## Roadmap

| Release | Scope |
|---|---|
| **v0.1.0** | Terminal lifecycle, cells, colors, alpha blending, diff renderer |
| v0.2.0 | Renderable tree, boxes and borders, first layout subset |
| v0.3.0 | Keyboard decoding, focus, text input |
| v0.4.0 | Scrolling, selection, mouse |
| v0.5.0 | Demo app, compatibility notes, performance pass |

## Known limitations (v0.1.0)

- Each rune occupies one cell; wide characters (CJK, emoji) and grapheme clusters are not handled yet.
- Truecolor is assumed; there is no terminal capability detection yet.
- No hyperlinks, images or cursor control.
- Input handling covers Ctrl+C only.

## License

MIT. See [LICENSE](LICENSE). Includes code ported from OpenTUI (MIT, © 2025 opentui).
