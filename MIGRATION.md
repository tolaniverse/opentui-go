# Migrating from v0.20.0 to typed widget handles

These changes shipped in **v0.21.0**.

Widget constructors now return `*TextNode`, `*InputNode`, `*TextareaNode`,
`*SelectNode`, `*TabSelectNode`, and `*ScrollBoxNode`. Each embeds `*Node`;
shared fields and handlers (`Style`, `ID`, `Rect`, `SetKeyHandler`, etc.) are
still available. `Box` and `NewNode` remain layout containers returning
`*Node`.

## Keep the widget handle for widget operations

```go
root := opentui.NewNode(opentui.Style{})
editor := opentui.Input(opentui.InputOptions{}) // *opentui.InputNode
editor.SetValue("hello") // input-only method
root.Add(editor)         // accepts NodeHandle
// renderer.Focus(editor) also accepts NodeHandle after attaching root.
```

Widget-only methods no longer compile on a plain `*Node` (the old calls could
silently no-op on an unrelated node). If a widget is stored in a struct, change
its field type from `*opentui.Node` to the corresponding handle. `Input` and
`Textarea` each have `Value`, `SetValue`, `InsertText`, `Submit`, `Undo` and
`Redo`. The cursor methods are specific to each. `Select` and `TabSelect`
share selection methods, but filtering belongs only to `SelectNode` and tab
width only to `TabSelectNode`.

## Trees and slices

`NewNode`, `Box`, `ScrollBox`, `Node.Add`, `Node.Remove`, `Renderer.Focus`,
`Draw`, `Paint`, and `ComputeLayout` accept `NodeHandle`, so mixed widgets and
layout nodes work directly. `Node.Children`, `Node.Find`, `Node.Parent`, `Renderer.Focused`, and
`Renderer.FocusNext` still return `*Node` because they can refer to any widget.
Use `AsInput(node)`, `AsSelect(node)`, etc. to recover a typed handle when you
know the expected kind; they return nil on a mismatch.

Go cannot expand a `[]*Node` as `...NodeHandle`. For a dynamically built slice,
use `opentui.NodeHandles(nodes)...`; a slice of any single widget-handle type
works too. `NodeHandles` allocates one result slice.

The terminal lifecycle and draw callback are unchanged. This is a source API
break, not a change in keyboard or painting behavior.
