# opentui-go

A pure-Go port of [OpenTUI](https://github.com/anomalyco/opentui), a library for building terminal user interfaces.

Upstream OpenTUI pairs a Zig engine with a TypeScript API. This port reimplements both layers in Go, with no cgo and no Zig.

> **Status: v0.22.0, early.** It includes mouse routing, scrolling, text selection, clipboard copy, keyboard decoding, focus management, grapheme-aware single-line and multiline text input, renderable trees, Yoga-compatible flex layout, Unicode-aware text rendering with soft wrapping, diff rendering, resize handling, and clean shutdown.
> macOS, Linux, and Windows. BSD builds; it is not in CI. Requires Go 1.25 or later; CI tests the oldest supported Go and the latest release.

Ported from upstream commit [`e4ad586`](https://github.com/anomalyco/opentui/tree/e4ad5867eae1051a2c1877b08d88bbcaf12b7209).
See [compatibility and missing features](COMPATIBILITY.md) before treating it as an upstream replacement.
See [the changelog and API stability policy](CHANGELOG.md) and [security reporting](SECURITY.md).

Since v0.21.0, widget constructors return typed handles and widget-only
methods no longer live on `*Node`. Callers of v0.20.0 or earlier need to migrate;
see [the migration guide](MIGRATION.md). Do not rely on v0.x source compatibility.

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
| `go run ./examples/color` | Detected color mode (truecolor, 256, 16, or none) |
| `go run ./examples/styled` | Rich text: bold, italic, underlined and colored spans, an OSC 8 hyperlink |
| `go run ./examples/select` | A `Select` list with descriptions, keyboard/mouse/wheel navigation, and a filter field |
| `go run ./examples/tabs` | A `TabSelect` bar with descriptions, overflow arrows, truncated names, and a panel that follows the selected tab |
| `go run ./examples/wine` | An animated 3D scene, ray marched on the CPU and drawn as half-block pixels: glowing glasses fill with poured wine, rise and clink, and spill a few drops onto the table. Space pauses, ←/→ orbit |
| `go run ./examples/tasks` | A usable local task list: add, edit, complete, filter, and persist across runs |

Press Ctrl+C to quit; the terminal is restored.

The tasks app stores JSON at `os.UserConfigDir()/opentui-go/tasks.json`; pass `-data /path/to/tasks.json` to use a different file. In the list, use Up/Down to move, Enter to toggle completion, `a` to add, `e` to edit, and `/` to filter. Enter saves an edited title; Escape cancels. Tab cycles focus. It writes a temporary file in the same directory and renames it after syncing; a failed save leaves the in-memory list unchanged. It is a dogfooding app, not a new library API.

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

`Run` handles decoded keyboard and bracketed-paste input, Ctrl+C, SIGINT, SIGTERM, SIGHUP, and resizes. On Windows, resize is polled (there is no SIGWINCH) and the console is switched into virtual-terminal mode. It restores the terminal on every exit path, including a panic in your draw function.

## Keyboard and focus

- `KeyEvent` reports printable keys, controls, arrows, function keys, modifiers, and press/repeat/release events from traditional terminal sequences and CSI-u/Kitty input.
- `Renderer.SetKeyHandler` receives keys before the focused node. `PreventDefault` suppresses built-in Tab and Ctrl+C behavior; `StopPropagation` stops focused-node delivery.
- `Renderer.Focus`, `Blur`, `Focused`, and `FocusNext` manage focus. Tab and Shift+Tab traverse visible focusable nodes in depth-first order and wrap.
- `Textarea` is a multiline editor. Enter inserts a newline and Alt/Meta+Enter submits; arrows cross lines, and Up/Down keep the terminal-cell column; Ctrl+A/E go to the line start/end, Home/End to the buffer start/end; Backspace/Delete/Ctrl+W/K join lines at their edges. Paste keeps newlines (CRLF/CR become LF) and expands tabs to 4 spaces. `CursorPosition`/`SetCursorPosition` use a line index plus a rune offset snapped to a grapheme boundary.
- `Textarea` selection: click places the cursor; drag, Shift+click, or Shift with any movement key selects; double-click selects a word; Super+A selects all. Typing, pasting, or any delete key replaces the selection; plain Left/Right collapse it. `SelectedText`, `SelectAll`, and `OnSelect` (called when a mouse selection ends, e.g. to copy) expose it. The wheel scrolls the view without moving the cursor (and bubbles at the ends); the next key or click brings the cursor back into view.
- `Input` supports cursor movement, Home/End, Backspace/Delete, Ctrl+A/E/U/K/W, bracketed paste, horizontal scrolling, placeholders, length limits, and input/change/submit callbacks.
- `Input` and `Textarea` share upstream's editing keys: word movement with Ctrl/Alt+Left/Right and Alt+B/F, word deletion with Ctrl+W, Alt/Ctrl+Backspace, Alt+D and Alt/Ctrl+Delete (word boundaries are Unicode line-break opportunities, as upstream), and Undo/Redo with Ctrl+- / Ctrl+. and Super+Z / Super+Shift+Z. Because most terminals can only send those with the kitty keyboard protocol, Ctrl+Z / Ctrl+Y (and Ctrl+Shift+Z) also work. Typing undoes by word; runs of Backspace or Delete undo together; `SetValue` clears the history, as upstream's `setText`. `InputNode.Undo` / `TextareaNode.Undo` and `Redo` do the same from code.
- `Input` moves and deletes whole grapheme clusters, scrolls in terminal cells, and enforces MinLength/MaxLength in clusters. `Cursor`/`SetCursor` retain rune offsets for API compatibility; `SetCursor` snaps to the prior grapheme boundary.

## Mouse, scrolling, and selection

- Mouse reporting uses SGR mode plus motion mode by default. Events bubble from the deepest hit node and support hover, drag capture, drop, and left-click focus.
- `ScrollBox` clips content, handles wheel and arrow navigation, supports programmatic offsets and sticky-end follow, and paints overlay scrollbars. Drag a scrollbar thumb to scroll, or click the track to page. Box padding is applied to the scrolled content, as upstream.
- Dragging selectable `Text` creates a renderer-owned selection. Double-click selects a word and triple-click selects a line. Dragging past a `ScrollBox` edge auto-scrolls it, faster the further out the pointer is.
- `Renderer.CopyToClipboard` sends text to the terminal's clipboard with OSC 52, so it works over SSH. Terminals may disable or size-limit OSC 52 and never confirm it.
- `Text` measures, paints, clips, and selects Unicode grapheme clusters by terminal-cell width (including CJK and emoji sequences); plain ASCII keeps a fast path.
- `Select` is a port of upstream's list: options with name, description and value; Up/K, Down/J, Shift+Up/Down (fast step 5), Enter; `OnChange`/`OnSelect` as upstream's `selectionChanged`/`itemSelected`; selection kept centered; optional scroll indicator, wrapping, item spacing. Beyond upstream: a click selects (a click on the selected item chooses it, and the clicked item stays under the pointer), the wheel moves the selection, `SetFilter` narrows by name or description, and it measures to its content when unsized. ASCII-art fonts are not supported.
- `TabSelect` is a port of upstream's horizontal tab bar: fixed-width tabs (`TabWidth`, default 20), Left/`[` and Right/`]` move, Enter chooses; the selected tab is filled and underlined with `▬`, and its description is shown below. Names that do not fit end in `…`. When tabs overflow, the selection stays centered and `‹`/`›` arrows mark hidden tabs. `HideUnderline`, `HideDescription`, `HideScrollArrows` and `WrapSelection` match upstream's options; the height (1–3 rows) follows them. As upstream, a move that cannot happen at an end does not fire `OnChange`. It shares `SelectedIndex`, `SetSelectedIndex`, `SelectedOption`, `MoveSelection`, `SelectCurrent` and `SetSelectOptions` with `Select`. Beyond upstream: a click selects a tab (a click on the selected tab chooses it, and the clicked tab stays under the pointer) and the wheel moves between tabs.
- `Styled(opts, spans...)` and `TextNode.SetSpans` show rich text, the counterpart of upstream's styled-text chunks: each `Span` has its own FG, BG, attributes and optional hyperlink, and spans may contain newlines. Build spans with `S("text").Bold().Fg(c)` or `Link(url, text)`. Styles survive wrapping and selection; a span boundary inside a grapheme cluster gives the cluster the first span's style. `SetText` returns to a single style.
- Hyperlinks use OSC 8 and are sent only to terminals known to support them, as upstream: kitty, Ghostty, WezTerm, Alacritty, foot, iTerm2, VS Code, Zed, mintty, VTE ≥ 0.52.2, and WSL in Windows Terminal; never inside tmux/screen/zellij. `FORCE_HYPERLINK` or `Options.Hyperlinks` overrides. Link URLs have control bytes removed.
- `TextOptions.Wrap` and `TextareaOptions.Wrap` soft-wrap lines: `WrapWord` breaks at Unicode line-break opportunities (falling back to character breaks for over-long words), `WrapChar` between any grapheme clusters. Wrapped `Text` grows taller in layout and reflows on resize; trailing spaces hang past the edge. Selecting across a soft wrap adds no newline. A wrapping `Textarea` keeps its last column for the cursor, and Up/Down move by visual row. The zero value, `WrapDefault`, wraps `Textarea` by word, as upstream's editor does, and leaves `Text` unwrapped (upstream `Text` wraps by word); pass `WrapNone` for an editor that scrolls long lines sideways.
- `TextOptions.Truncate` keeps the start and end of each overflowing row around `...` (or leaves an overflowing row blank when the viewport is at most three cells wide). Truncation preserves the original text and styled spans. `Style{Direction: Row, FlexWrap: FlexWrapLines, Gap: 1}` wraps children onto additional flex lines; the same field works for columns. `FlexNoWrap` remains the zero-value default. `WrapReverse` and `AlignContent` are not implemented.

## Testing

```bash
go test -race ./...                         # library unit tests
(go test -run Example -v .)                # executable package documentation
(cd tools/ptycheck && go test -race ./...)  # run examples in a real pty
scripts/release-check.sh [ref]              # everything, on a clean checkout of ref
```

`tools/ptycheck` is a separate module, so the library keeps no test-only dependencies. On Linux and macOS it runs every example in a pseudo-terminal, drives it with real key, paste, and mouse sequences, and checks an emulated screen, including clean exit and terminal-mode cleanup. On Windows the same harness uses a pseudoconsole (ConPTY) for three smoke tests (hello, input, textarea wrap and resize) and the shared tasks-app workflow. ConPTY re-encodes output, so those tests check visible text, not exact escape sequences. Run `scripts/release-check.sh` before tagging; it checks only what is committed, and cross-compiles for Windows.

`go test -run '^$' -bench . -benchmem .` runs the benchmarks: frame encoding (unchanged, one-cell, full), layout and paint of plain and wrapped documents, a full Unicode screen, and editor keystrokes. Frame encoding does not allocate (a test enforces it). On an Apple M1 (Go 1.26.2, 80×24), v0.16.0 encodes an unchanged frame in ~7.5 µs and a full repaint in ~24 µs, down from ~15 µs and ~34 µs in v0.15.0, by storing a cell's grapheme cluster and link in one interned handle; drawing is statistically unchanged (benchstat, n=10). These are local Go measurements, not comparisons with upstream Zig; rerun on your machine for relevant numbers.

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
| **v0.12.0** | Color capability detection: truecolor, 256-color, 16-color, and `NO_COLOR` |
| **v0.13.0** | Styled text spans (colors, attributes, OSC 8 hyperlinks) |
| **v0.14.0** | `Select` list widget with mouse, wheel and filtering |
| **v0.15.0** | `Textarea` wraps by word by default, as upstream (`WrapDefault`) |
| **v0.16.0** | Performance: 2× faster frame diff, allocation-free frame encoding (breaking: `Cell.Grapheme`/`Cell.Link` are methods) |
| **v0.17.0** | Windows console support (virtual-terminal mode, polled resize) and ConPTY smoke tests in CI |
| **v0.18.0** | Dogfooding: persistent local tasks app, end-to-end add/edit/filter/complete/restart workflow on Unix and Windows |
| **v0.19.0** | `TabSelect` horizontal tab bar with ellipsis truncation, overflow arrows, mouse and wheel |
| **v0.20.0** | Go 1.25 minimum (was 1.26.2); CI on oldest and latest Go; staticcheck and govulncheck in CI |
| **v0.21.0** | Typed widget handles (breaking; see `MIGRATION.md`); package docs and runnable examples |
| **v0.22.0** | `Text` truncation with ellipsis; forward flex wrap (`Style.FlexWrap`) |

## Layout

`Style` supports row/column directions (including reverse), grow and shrink, basis, fixed and percentage dimensions, min/max constraints, gap, padding, margins, justify/align, and absolute positioning. Layout is a pure-Go port of the supported Yoga 3.2.1 algorithm and is checked against 461 generated real-Yoga cases, including forward flex wrap. It also carries OpenTUI's flex-shrink residue fix.

## Known limitations (v0.22.0)

- `Input` is still single-line and its public cursor offsets are rune indices; editing itself uses grapheme boundaries. Terminal width calculations use `uniseg` defaults and may differ from a particular terminal's emoji width settings.
- Color mode is detected from `TERM`, `COLORTERM`, `WT_SESSION`, and `NO_COLOR`. It does not query the terminal. Unknown terminals still get truecolor, as upstream does. `Options.Color` can force a mode.
- No images or terminal cursor control. `Textarea` and `Input` contents are plain text (no spans).
- Clipboard copy is write-only OSC 52; there is no clipboard read and no tmux/screen passthrough wrapping yet.
- `Textarea` has no triple-click line selection; holding a drag still past the edge does not keep scrolling (each pointer move scrolls one row). Without wrapping, all lines share one horizontal scroll offset. Ctrl+A/E and Home/End act on logical lines and the buffer, not visual rows.
- Windows resize is noticed by polling, about every 100 ms. Windows CI runs three ConPTY smoke tests, not the full Unix PTY suite; exact escape sequences are not checked there because ConPTY re-encodes them.
- No flex wrapping, align-content, baseline alignment, aspect ratio, percentage padding/margins, or auto margins yet.
- Word-mode line breaking follows Unicode UAX #14 defaults via `uniseg`, without hyphenation or language-specific tailoring.

## License

MIT. See [LICENSE](LICENSE). Includes code ported from OpenTUI (MIT, © 2025 opentui).
