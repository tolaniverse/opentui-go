package opentui

// TextNode is a text renderable with text and span operations.
type TextNode struct{ *Node }

// InputNode is a single-line editor.
type InputNode struct{ *Node }

// TextareaNode is a multiline editor.
type TextareaNode struct{ *Node }

// SelectNode is a filterable vertical option list.
type SelectNode struct{ *Node }

// TabSelectNode is a horizontal option bar.
type TabSelectNode struct{ *Node }

// ScrollBoxNode is a scrollable viewport.
type ScrollBoxNode struct{ *Node }

// AsText recovers a text handle from a node found by ID; nil if it is not Text.
func AsText(n *Node) *TextNode {
	if n == nil || n.text == nil {
		return nil
	}
	return &TextNode{n}
}

// AsInput recovers an input handle from a node found by ID; nil if it is not Input.
func AsInput(n *Node) *InputNode {
	if n == nil || n.input == nil {
		return nil
	}
	return &InputNode{n}
}

// AsTextarea recovers a textarea handle from a node found by ID.
func AsTextarea(n *Node) *TextareaNode {
	if n == nil || n.textarea == nil {
		return nil
	}
	return &TextareaNode{n}
}

// AsSelect recovers a select handle from a node found by ID.
func AsSelect(n *Node) *SelectNode {
	if n == nil || n.list == nil {
		return nil
	}
	return &SelectNode{n}
}

// AsTabSelect recovers a tab-select handle from a node found by ID.
func AsTabSelect(n *Node) *TabSelectNode {
	if n == nil || n.tabs == nil {
		return nil
	}
	return &TabSelectNode{n}
}

// AsScrollBox recovers a scroll-box handle from a node found by ID.
func AsScrollBox(n *Node) *ScrollBoxNode {
	if n == nil || n.scroll == nil {
		return nil
	}
	return &ScrollBoxNode{n}
}

// AsNode returns the underlying node, or nil for a nil handle.
func (n *TextNode) AsNode() *Node {
	if n == nil {
		return nil
	}
	return n.Node
}
func (n *InputNode) AsNode() *Node {
	if n == nil {
		return nil
	}
	return n.Node
}
func (n *TextareaNode) AsNode() *Node {
	if n == nil {
		return nil
	}
	return n.Node
}
func (n *SelectNode) AsNode() *Node {
	if n == nil {
		return nil
	}
	return n.Node
}
func (n *TabSelectNode) AsNode() *Node {
	if n == nil {
		return nil
	}
	return n.Node
}
func (n *ScrollBoxNode) AsNode() *Node {
	if n == nil {
		return nil
	}
	return n.Node
}

// SetText replaces plain text and discards styled spans.
func (n *TextNode) SetText(s string) { n.Node.setText(s) }

// TextContent returns the current plain text.
func (n *TextNode) TextContent() string { return n.Node.textContent() }

// SetSpans replaces the styled text spans.
func (n *TextNode) SetSpans(s ...Span) { n.Node.setSpans(s...) }

// Spans returns a copy of the styled spans.
func (n *TextNode) Spans() []Span { return n.Node.spans() }

// Value returns the current editor contents.
func (n *InputNode) Value() string { return n.Node.value() }

// SetValue replaces the editor contents and clears undo history.
func (n *InputNode) SetValue(s string) { n.Node.setValue(s) }

// Cursor returns the single-line cursor offset in runes.
func (n *InputNode) Cursor() int { return n.Node.cursor() }

// SetCursor moves the single-line cursor to a grapheme boundary.
func (n *InputNode) SetCursor(i int) { n.Node.setCursor(i) }

// InsertText inserts text at the cursor, reporting whether it changed.
func (n *InputNode) InsertText(s string) bool { return n.Node.insertText(s) }

// Submit invokes the submit callback when the value is valid.
func (n *InputNode) Submit() bool { return n.Node.submit() }

// Undo reverts the last edit group.
func (n *InputNode) Undo() bool { return n.Node.undo() }

// Redo reapplies the last undone edit group.
func (n *InputNode) Redo() bool { return n.Node.redo() }

// Value returns the current editor contents.
func (n *TextareaNode) Value() string { return n.Node.value() }

// SetValue replaces the editor contents and clears undo history.
func (n *TextareaNode) SetValue(s string) { n.Node.setValue(s) }

// InsertText inserts text at the cursor, reporting whether it changed.
func (n *TextareaNode) InsertText(s string) bool { return n.Node.insertText(s) }

// Submit invokes the submit callback when the value is valid.
func (n *TextareaNode) Submit() bool { return n.Node.submit() }

// CursorPosition returns the textarea line and rune column.
func (n *TextareaNode) CursorPosition() (int, int) { return n.Node.cursorPosition() }

// SetCursorPosition moves the textarea cursor to a line and rune column.
func (n *TextareaNode) SetCursorPosition(line, col int) { n.Node.setCursorPosition(line, col) }

// LineCount returns the number of logical lines in the textarea.
func (n *TextareaNode) LineCount() int { return n.Node.lineCount() }

// SelectedText returns the textarea selection or an empty string.
func (n *TextareaNode) SelectedText() string { return n.Node.selectedText() }

// SelectAll selects all textarea text.
func (n *TextareaNode) SelectAll() { n.Node.selectAll() }

// Undo reverts the last edit group.
func (n *TextareaNode) Undo() bool { return n.Node.undo() }

// Redo reapplies the last undone edit group.
func (n *TextareaNode) Redo() bool { return n.Node.redo() }

// SetSelectOptions replaces the options while retaining a valid selection.
func (n *SelectNode) SetSelectOptions(o []SelectOption) { n.Node.setSelectOptions(o) }

// SetFilter filters Select options by name or description.
func (n *SelectNode) SetFilter(s string) { n.Node.setFilter(s) }

// VisibleOptions returns the number of options matching the filter.
func (n *SelectNode) VisibleOptions() int { return n.Node.visibleOptions() }

// SelectedIndex returns the index of the selected option.
func (n *SelectNode) SelectedIndex() int { return n.Node.selectedIndex() }

// SelectedOption returns the current option, if one is visible.
func (n *SelectNode) SelectedOption() (SelectOption, bool) { return n.Node.selectedOption() }

// SetSelectedIndex selects an option by index and notifies listeners.
func (n *SelectNode) SetSelectedIndex(i int) { n.Node.setSelectedIndex(i) }

// MoveSelection moves selection by the given number of items.
func (n *SelectNode) MoveSelection(i int) { n.Node.moveSelection(i) }

// SelectCurrent invokes the selection callback for the current option.
func (n *SelectNode) SelectCurrent() bool { return n.Node.selectCurrent() }

// SetSelectOptions replaces the options while retaining a valid selection.
func (n *TabSelectNode) SetSelectOptions(o []SelectOption) { n.Node.setSelectOptions(o) }

// SelectedIndex returns the index of the selected option.
func (n *TabSelectNode) SelectedIndex() int { return n.Node.selectedIndex() }

// SelectedOption returns the current option, if one is visible.
func (n *TabSelectNode) SelectedOption() (SelectOption, bool) { return n.Node.selectedOption() }

// SetSelectedIndex selects an option by index and notifies listeners.
func (n *TabSelectNode) SetSelectedIndex(i int) { n.Node.setSelectedIndex(i) }

// MoveSelection moves selection by the given number of items.
func (n *TabSelectNode) MoveSelection(i int) { n.Node.moveSelection(i) }

// SelectCurrent invokes the selection callback for the current option.
func (n *TabSelectNode) SelectCurrent() bool { return n.Node.selectCurrent() }

// SetTabWidth sets the cell width of each tab.
func (n *TabSelectNode) SetTabWidth(i int) { n.Node.setTabWidth(i) }

// TabWidth returns the cell width of each tab.
func (n *TabSelectNode) TabWidth() int { return n.Node.tabWidth() }

// ScrollBy moves the viewport by the given cell deltas.
func (n *ScrollBoxNode) ScrollBy(dx, dy int) bool { return n.Node.scrollBy(dx, dy) }

// ScrollTo moves the viewport to an absolute cell offset.
func (n *ScrollBoxNode) ScrollTo(x, y int) bool { return n.Node.scrollTo(x, y) }

// ScrollOffset returns the current horizontal and vertical offsets.
func (n *ScrollBoxNode) ScrollOffset() (int, int) { return n.Node.scrollOffset() }

// ScrollSize returns the laid-out content size.
func (n *ScrollBoxNode) ScrollSize() (int, int) { return n.Node.scrollSize() }

// ViewportRect returns the visible content rectangle.
func (n *ScrollBoxNode) ViewportRect() Rect { return n.Node.viewportRect() }
