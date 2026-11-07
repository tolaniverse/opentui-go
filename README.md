# opentui-go

A pure-Go port of [OpenTUI](https://github.com/anomalyco/opentui), a library for building terminal user interfaces.

Upstream OpenTUI pairs a Zig engine with a TypeScript API. This port reimplements both layers in Go, with no cgo and no Zig.

> **Status: v0.11.0, early.** It includes mouse routing, scrolling, text selection, clipboard copy, keyboard decoding, focus management, grapheme-aware single-line and multiline text input, renderable trees, Yoga-compatible flex layout, Unicode-aware text rendering with soft wrapping, diff rendering, resize handling, and clean shutdown.
> Unix only (macOS, Linux, BSD).

Ported from upstream commit [`e4ad586`](https://github.com/anomalyco/opentui/tree/e4ad5867eae1051a2c1877b08d88bbcaf12b7209).
See [compatibility and missing features](COMPATIBILITY.md) before treating it as an upstream replacement.

## Try it

Each example focuses on one part of the library:

| Example | Shows |
|---|---|
| `go run ./examples/hello` | The smallest program: a bordered box of text |
| `go run ./examples/layout` | Flex layout, boxes, borders, clipping, live updates from a goroutine, resize |
| `go run ./examples/input` | Text inputs, Tab / Shift+Tab focus, bracketed paste, submit |
| `go run ./examples/document` | `ScrollBox` (wheel, keys, draggable scrollbar), selection with auto-scroll, clipboard copy |
| `go run ./examples/mouse` | Hover, clicks with bubbling, drag-and-drop between zones |
| `go run ./examples/unicode` | Wide characters, combining marks, emoji grapheme clusters |
| `go run ./examples/textarea` | Multiline editing, multi-line paste, mouse cursor placement, drag/Shift/double-click selection with copy, wheel scroll, submit |
| `go run ./examples/wrap` | Word-wrapped text that reflows on resize, a wrapping editor, selection across soft wraps |

Press Ctrl+C to quit; the terminal is restored.

## Use it

```go
r, err := opentui.New(opentui.Options{Background: opentui.MustHex("#1e1e2e")})
if err != nil { log.Fatal(err) }

document := opentui.ScrollBox(opentui.ScrollBoxOptions{
	BoxOptions: opentui.BoxOptions{Style: opentui.Style{FlexGrow: 1}},
}, opentui.Text("A scrollable, selectable document", opentui.TextOptions{}))
r.Root().Add(document)
r.Focus(document)
err = r.Run(context.Background(), nil)
```

`Run` handles decoded keyboard and bracketed-paste input, Ctrl+C, SIGINT, SIGTERM, SIGHUP, and resizes. It restores the terminal on every exit path, including a panic in your draw function.

## Keyboard and focus

- `KeyEvent` reports printable keys, controls, arrows, function keys, modifiers, and press/repeat/release events from traditional terminal sequences and CSI-u/Kitty input.
- `Renderer.SetKeyHandler` receives keys before the focused node. `PreventDefault` suppresses built-in Tab and Ctrl+C behavior; `StopPropagation` stops focused-node delivery.
- `Renderer.Focus`, `Blur`, `Focused`, and `FocusNext` manage focus. Tab and Shift+Tab traverse visible focusable nodes in depth-first order and wrap.
- `Textarea` is a multiline editor. Enter inserts a newline and Alt/Meta+Enter submits; arrows cross lines, and Up/Down keep the terminal-cell column; Ctrl+A/E go to the line start/end, Home/End to the buffer start/end; Backspace/Delete/Ctrl+W/K join lines at their edges. Paste keeps newlines (CRLF/CR become LF) and expands tabs to 4 spaces. `CursorPosition`/`SetCursorPosition` use a line index plus a rune offset snapped to a grapheme boundary.
- `Textarea` selection: click places the cursor; drag, Shift+click, or Shift with any movement key selects; double-click selects a word; Super+A selects all. Typing, pasting, or any delete key replaces the selection; plain Left/Right collapse it. `SelectedText`, `SelectAll`, and `OnSelect` (called when a mouse selection ends, e.g. to copy) expose it. The wheel scrolls the view without moving the cursor (and bubbles at the ends); the next key or click brings the cursor back into view.
- `Input` supports cursor movement, Home/End, Backspace/Delete, Ctrl+A/E/U/K/W, bracketed paste, horizontal scrolling, placeholders, length limits, and input/change/submit callbacks.
- `Input` and `Textarea` share upstream's editing keys: word movement with Ctrl/Alt+Left/Right and Alt+B/F, word deletion with Ctrl+W, Alt/Ctrl+Backspace, Alt+D and Alt/Ctrl+Delete (word boundaries are Unicode line-break opportunities, as upstream), and Undo/Redo with Ctrl+- / Ctrl+. and Super+Z / Super+Shift+Z. Because most terminals can only send those with the kitty keyboard protocol, Ctrl+Z / Ctrl+Y (and Ctrl+Shift+Z) also work. Typing undoes by word; runs of Backspace or Delete undo together; `SetValue` clears the history, as upstream's `setText`. `Node.Undo` / `Node.Redo` do the same from code.
- `Input` moves and deletes whole grapheme clusters, scrolls in terminal cells, and enforces MinLength/MaxLength in clusters. `Cursor`/`SetCursor` retain rune offsets for API compatibility; `SetCursor` snaps to the prior grapheme boundary.

## Mouse, scrolling, and selection

- Mouse reporting uses SGR mode plus motion mode by default. Events bubble from the deepest hit node and support hover, drag capture, drop, and left-click focus.
- `ScrollBox` clips content, handles wheel and arrow navigation, supports programmatic offsets and sticky-end follow, and paints overlay scrollbars. Drag a scrollbar thumb to scroll, or click the track to page. Box padding is applied to the scrolled content, as upstream.
- Dragging selectable `Text` creates a renderer-owned selection. Double-click selects a word and triple-click selects a line. Dragging past a `ScrollBox` edge auto-scrolls it, faster the further out the pointer is.
- `Renderer.CopyToClipboard` sends text to the terminal's clipboard with OSC 52, so it works over SSH. Terminals may disable or size-limit OSC 52 and never confirm it.
- `Text` measures, paints, clips, and selects Unicode grapheme clusters by terminal-cell width (including CJK and emoji sequences); plain ASCII keeps a fast path.
- `TextOptions.Wrap` and `TextareaOptions.Wrap` soft-wrap lines: `WrapWord` breaks at Unicode line-break opportunities (falling back to character breaks for over-long words), `WrapChar` between any grapheme clusters. Wrapped `Text` grows taller in layout and reflows on resize; trailing spaces hang past the edge. Selecting across a soft wrap adds no newline. A wrapping `Textarea` keeps its last column for the cursor, and Up/Down move by visual row. The zero value is `WrapNone`; upstream defaults to word wrapping.

## Testing

```bash
go test -race ./...                         # library unit tests
(cd tools/ptycheck && go test -race ./...)  # run examples in a real pty
scripts/release-check.sh [ref]              # everything, on a clean checkout of ref
```

`tools/ptycheck` is a separate module, so the library keeps no test-only dependencies. It runs every example in a pseudo-terminal, drives it with real key, paste, and mouse sequences, and checks an emulated screen, including clean exit and terminal-mode cleanup. Run `scripts/release-check.sh` before tagging; it checks only what is committed.

`go test -run '^$' -bench 'Benchmark(EncodeFrame|DrawDocument)' -benchmem .` measures unchanged, one-cell, and full-frame diffs plus layout/paint of a 200-row scroll document. On an Apple M1 (Go 1.26.2, 80×24), batching ANSI numeric sequences reduced full-frame encoding from ~35.8 µs / 720 allocations to ~25.5 µs / 216 allocations per frame. These are local Go measurements, not comparisons with upstream Zig; rerun on your machine for relevant numbers.

## Roadmap

| Release | Scope |
|---|---|
| **v0.1.0** | Terminal lifecycle, cells, colors, alpha blending, diff renderer |
| **v0.2.0** | Renderable tree, boxes, borders, clipping, Yoga-compatible basic layout |
| **v0.3.0** | Keyboard decoding, focus, text input |
| **v0.4.0** | Scrolling, selection, mouse |
| **v0.5.0** | Focused examples, scrollbar dragging, selection auto-scroll, clipboard copy, compatibility notes, performance pass |
| **v0.6.0** | Grapheme-aware `Text` measurement, painting, clipping, selection, and frame diff; Unicode example |
| **v0.7.0** | Grapheme-aware `Input` navigation, editing, limits, and horizontal scrolling |
| **v0.8.0** | Multiline `Textarea` with grapheme-aware editing, multi-line paste, and two-axis scrolling |
| **v0.9.0** | Soft wrapping (word and char) for `Text` and `Textarea`; selection across soft wraps |
| **v0.10.0** | `Textarea` mouse support: click, drag, double-click, Shift selection, wheel scroll; typing replaces the selection |
| **v0.11.0** | Undo/redo and word movement/deletion in `Input` and `Textarea` |

## Layout

`Style` supports row/column directions (including reverse), grow and shrink, basis, fixed and percentage dimensions, min/max constraints, gap, padding, margins, justify/align, and absolute positioning. Layout is a pure-Go port of the supported Yoga 3.2.1 algorithm and is checked against 410 generated real-Yoga cases. It also carries OpenTUI's flex-shrink residue fix.

## Known limitations (v0.11.0)

- `Input` is still single-line and its public cursor offsets are rune indices; editing itself uses grapheme boundaries. Terminal width calculations use `uniseg` defaults and may differ from a particular terminal's emoji width settings.
- Truecolor is assumed; there is no terminal capability detection yet.
- No hyperlinks, images or cursor control.
- Clipboard copy is write-only OSC 52; there is no clipboard read and no tmux/screen passthrough wrapping yet.
- `Textarea` has no triple-click line selection; holding a drag still past the edge does not keep scrolling (each pointer move scrolls one row). Without wrapping, all lines share one horizontal scroll offset. Ctrl+A/E and Home/End act on logical lines and the buffer, not visual rows.
- No flex wrapping, align-content, baseline alignment, aspect ratio, percentage padding/margins, or auto margins yet.
- `Text` supports plain text only; styled spans arrive in a later release. Word-mode line breaking follows Unicode UAX #14 defaults via `uniseg`, without hyphenation or language-specific tailoring.

## License

MIT. See [LICENSE](LICENSE). Includes code ported from OpenTUI (MIT, © 2025 opentui).
