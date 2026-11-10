// Package opentui builds terminal interfaces with a pure-Go layout, input,
// and diff-rendering engine. It requires Go 1.25 or newer.
//
// A Renderer owns the terminal while Run is active. Add boxes, text and
// controls to its Root before Run; use Update to mutate the tree from another
// goroutine. Run restores terminal state on return, including after a panic
// in the draw callback. The renderer's draw callback runs after the tree is
// painted and can use Buffer.Set for custom cell graphics.
//
//	r, err := opentui.New(opentui.Options{})
//	if err != nil { return err }
//	r.Root().Add(opentui.Text("Hello", opentui.TextOptions{}))
//	return r.Run(ctx, nil)
//
// OpenTUI Go is a partial, idiomatic port, not a drop-in replacement for
// upstream TypeScript/Zig OpenTUI. See COMPATIBILITY.md for the supported
// subset, and README.md for interactive examples and terminal requirements.
// The v0.x API can change before the v1.0 core is frozen.
package opentui
