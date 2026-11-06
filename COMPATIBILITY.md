# Upstream compatibility (v0.6.0)

This is a **partial, idiomatic Go port**, not a drop-in replacement for
OpenTUI's TypeScript or Zig APIs. The comparison is pinned to upstream commit
[`e4ad5867`](https://github.com/anomalyco/opentui/tree/e4ad5867eae1051a2c1877b08d88bbcaf12b7209).
There is no cgo, Zig runtime, or JavaScript runtime dependency.

| Area | Ported | Different or missing |
|---|---|---|
| Terminal and cells | Alternate screen, raw input, signal/resize cleanup, clipping, color blending, grapheme-width-aware text cells and diff output, synchronized frames | Assumes truecolor; no capability negotiation, terminal-specific width override, hyperlink IDs, images, or native SIMD buffer layout. See [upstream buffer](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/native/src/buffer.zig) and [renderer](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/native/src/renderer.zig). |
| Layout and boxes | Flex direction, grow/shrink/basis, fixed/percentage dimensions, min/max, gap, padding, margin, alignment, absolute positioning, borders, titles, clipping | No flex wrap, align-content/baseline, aspect ratio, percentage padding/margins, or auto margins. The supported layout subset is checked against 410 Yoga-generated cases; that does not imply full Yoga parity. See [upstream Box](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/core/src/renderables/Box.ts). |
| Text and input | Grapheme-aware plain-text measurement, painting, clipping and selection; single-line input, keyboard and bracketed paste, focus traversal | No rich spans, wrapping, grapheme-aware input editing, multiline textarea, IME support, or upstream text-buffer APIs. See [Text](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/core/src/renderables/Text.ts), [Input](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/core/src/renderables/Input.ts), and [Textarea](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/core/src/renderables/Textarea.ts). |
| Mouse and scroll | SGR pointer events, hover/capture/drop, wheel and keyboard scroll, clipping, thumb dragging, track paging, selection auto-scroll | The scrolling and selection algorithms are simplified, not event-for-event upstream equivalents. See [upstream ScrollBox](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/core/src/renderables/ScrollBox.ts). |
| Clipboard | OSC 52 write to clipboard or primary selection | No clipboard read, capability detection, tmux/screen passthrough, or confirmation that the terminal accepted a copy. See [upstream clipboard API](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/core/src/lib/clipboard.ts) and [terminal encoding](https://github.com/anomalyco/opentui/blob/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/native/src/terminal.zig). |
| Components and bindings | `Node`, `Box`, `Text`, `Input`, `ScrollBox`, `Renderer` | No React/Solid bindings, code/diff/select widgets, animation system, or plugin API. See [upstream renderables](https://github.com/anomalyco/opentui/tree/e4ad5867eae1051a2c1877b08d88bbcaf12b7209/packages/core/src/renderables). |

The examples in `examples/` show the Go API by feature, rather than translating
TypeScript examples literally. The `tools/ptycheck` module exercises them in
a pseudo-terminal on Linux and macOS CI. `go test ./...` covers the library;
neither set of tests asserts complete upstream compatibility.
