# Changelog

This file records user-visible changes. Tagged releases and their full notes
are available on [GitHub](https://github.com/tolaniverse/opentui-go/releases).

## v0.22.0 — 2026-11-10

- Text: `TextOptions.Truncate` keeps whole-grapheme prefix and suffix around a
  three-cell ellipsis, with selection mapped to the original source.
- Layout: `Style.FlexWrap` supports forward row/column flex lines with gaps;
  `FlexNoWrap` remains the zero value. Yoga oracle cases cover the supported
  subset; wrap-reverse and align-content distribution remain unimplemented.

## v0.21.0 — 2026-11-10

- Breaking API: widget constructors return typed handles (`*TextNode`,
  `*InputNode`, `*TextareaNode`, `*SelectNode`, `*TabSelectNode`,
  `*ScrollBoxNode`). Widget-only methods moved off `*Node`.
  Tree/focus APIs accept `NodeHandle`; see [MIGRATION.md](MIGRATION.md).
- Added a half-block 3D wine example with visual clink and spill; refined its
  materials and lighting. This is an example, not a new graphics API.
- Added package documentation, runnable examples and a security policy.

## v0.20.0 — 2026-11-10

- Lowered the minimum supported Go version to 1.25.
- CI tests the oldest supported and latest stable Go on Linux, macOS and
  Windows; lint runs gofmt, staticcheck and govulncheck.

## v0.19.0 — 2026-11-10

- Added the `TabSelect` horizontal tab bar and its PTY example/test.

## v0.18.1 — 2026-11-09

- Corrected a flaky Windows input smoke-test assertion.

## v0.18.0 — 2026-11-09

- Added a persistent local tasks example with Unix PTY and Windows ConPTY
  end-to-end coverage.

Earlier tagged releases: [releases](https://github.com/tolaniverse/opentui-go/releases).

## Compatibility policy

Versions before v1.0 may contain breaking public API changes; they are called
out in release notes and this changelog. v1.0 will freeze the documented core
API under semantic versioning. Features marked missing in
[COMPATIBILITY.md](COMPATIBILITY.md) are not promised for v1.0; new widgets
can be added in later minor releases. Do not treat upstream OpenTUI API
compatibility as a guarantee.
