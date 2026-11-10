package opentui

// SetKeyHandler installs a renderer-wide key handler. It runs before the
// focused node and can call PreventDefault or StopPropagation.
func (r *Renderer) SetKeyHandler(fn func(*KeyEvent)) { r.keyHandler = fn }

// SetPasteHandler installs a renderer-wide bracketed-paste handler.
func (r *Renderer) SetPasteHandler(fn func(*PasteEvent)) { r.pasteHandler = fn }

// SetMouseHandler installs a renderer-wide mouse handler. It runs before routing.
func (r *Renderer) SetMouseHandler(fn func(*MouseEvent)) { r.mouseHandler = fn }

// Focus moves keyboard focus to n. It returns false when n is nil,
// non-focusable, hidden, or detached from this renderer's tree.
func (r *Renderer) Focus(handle NodeHandle) bool {
	var n *Node
	if handle != nil {
		n = handle.AsNode()
	}
	if n == nil || !n.Focusable || !r.focusCandidate(n) {
		return false
	}
	if r.focused == n {
		return true
	}
	r.blurFocused()
	r.focused = n
	n.focused = true
	if n.onFocus != nil {
		n.onFocus()
	}
	if n.focusHandler != nil {
		n.focusHandler()
	}
	return true
}

// Blur clears keyboard focus.
func (r *Renderer) Blur() { r.blurFocused() }

// Focused returns the currently focused node, or nil.
func (r *Renderer) Focused() *Node { return r.focused }

// FocusNext advances through focusable nodes in depth-first tree order.
// reverse implements Shift+Tab traversal. It wraps at both ends.
func (r *Renderer) FocusNext(reverse bool) *Node {
	var candidates []*Node
	var walk func(*Node, bool)
	walk = func(n *Node, hidden bool) {
		hidden = hidden || n.Hidden
		if hidden {
			return
		}
		if n.Focusable {
			candidates = append(candidates, n)
		}
		for _, child := range n.children {
			walk(child, hidden)
		}
	}
	walk(r.root, false)
	if len(candidates) == 0 {
		r.blurFocused()
		return nil
	}
	index := -1
	for i, n := range candidates {
		if n == r.focused {
			index = i
			break
		}
	}
	if reverse {
		index--
		if index < 0 {
			index = len(candidates) - 1
		}
	} else {
		index = (index + 1) % len(candidates)
	}
	r.Focus(candidates[index])
	return candidates[index]
}

func (r *Renderer) blurFocused() {
	old := r.focused
	if old == nil {
		return
	}
	r.focused = nil
	old.focused = false
	if old.onBlur != nil {
		old.onBlur()
	}
	if old.blurHandler != nil {
		old.blurHandler()
	}
}

func (r *Renderer) focusCandidate(n *Node) bool {
	seenRoot := false
	for cur := n; cur != nil; cur = cur.parent {
		if cur.Hidden {
			return false
		}
		if cur == r.root {
			seenRoot = true
			break
		}
	}
	return seenRoot
}

func (r *Renderer) validateFocus() {
	if r.focused != nil && (!r.focused.Focusable || !r.focusCandidate(r.focused)) {
		r.blurFocused()
	}
}

// dispatchInput runs on the renderer goroutine. It reports whether the
// default Ctrl+C action should stop Run.
func (r *Renderer) dispatchInput(input terminalEvent) bool {
	switch event := input.(type) {
	case keyInput:
		key := event.event
		if r.keyHandler != nil {
			r.keyHandler(key)
		}
		target := r.focused
		if !key.PropagationStopped() && !key.DefaultPrevented() && target != nil {
			if target.userKeyHandler != nil {
				target.userKeyHandler(key)
			}
			if !key.PropagationStopped() && !key.DefaultPrevented() && target.keyHandler != nil {
				target.keyHandler(key)
			}
		}
		if key.Type == KeyPress && !key.DefaultPrevented() {
			if key.Name == "tab" {
				r.FocusNext(key.Shift)
				key.PreventDefault()
			}
			if key.Ctrl && key.Name == "c" && !r.opts.IgnoreCtrlC {
				return true
			}
		}
	case pasteInput:
		paste := event.event
		if r.pasteHandler != nil {
			r.pasteHandler(paste)
		}
		target := r.focused
		if !paste.PropagationStopped() && !paste.DefaultPrevented() && target != nil {
			if target.userPasteHandler != nil {
				target.userPasteHandler(paste)
			}
			if !paste.PropagationStopped() && !paste.DefaultPrevented() && target.pasteHandler != nil {
				target.pasteHandler(paste)
			}
		}
	case mouseInput:
		r.dispatchMouse(event.event)
	}
	return false
}
