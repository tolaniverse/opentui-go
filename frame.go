package opentui

import "github.com/tolaniverse/opentui-go/ansi"

// FrameOptions controls how a frame is encoded.
type FrameOptions struct {
	// Force repaints every cell, ignoring what the terminal already shows.
	Force bool
	// Sync wraps the frame in synchronized-output markers (DEC mode 2026) so
	// supporting terminals show it all at once, with no tearing.
	Sync bool

	// colors encodes colors for the terminal's color mode; nil sends
	// truecolor.
	colors *colorEncoder
}

// FrameStats reports what one frame encode did.
type FrameStats struct {
	CellsUpdated int
	Started      bool // false means nothing changed and nothing was written
}

// encodeFrame writes the escape sequences that turn the screen from `current`
// into `next`, then copies the painted cells into `current`.
//
// It is the heart of the renderer: a port of the cell loop in
// `prepareRenderFrameWithWriter` (upstream renderer.zig). Key ideas:
//
//  1. Skip rows that are identical in both buffers.
//  2. Skip cells that are identical; only changed cells cost bytes.
//  3. Group neighbouring changed cells with the same style into a "run":
//     one cursor move and one set of color codes, then just characters.
//  4. Write nothing at all if nothing changed (the frame starts lazily).
//
// Both buffers must be the same size.
func encodeFrame(w ansi.Writer, current, next *Buffer, opts FrameOptions) FrameStats {
	var stats FrameStats
	begin := func() {
		if stats.Started {
			return
		}
		stats.Started = true
		if opts.Sync {
			w.WriteString(ansi.SyncBegin)
		}
		w.WriteString(ansi.HideCursor)
	}

	links := opts.colors != nil && opts.colors.links // checked once, not per cell
	width := next.width
	for y := 0; y < next.height; y++ {
		row := y * width
		curRow := current.cells[row : row+width]
		nextRow := next.cells[row : row+width]

		if !opts.Force && rowsEqual(curRow, nextRow) {
			continue
		}

		var style Cell // fg/bg/attr of the current run; Char unused
		runLength := 0
		// link is the hyperlink open in the terminal. It is closed whenever
		// the cursor is about to move away, so it never covers other cells.
		link := ""
		closeLink := func() {
			if link != "" {
				w.WriteString(ansi.LinkEnd)
				link = ""
			}
		}

		for x := range nextRow {
			cell := nextRow[x]
			if cell.Continuation {
				if opts.Force || curRow[x] != cell {
					curRow[x] = cell // emitted with the preceding wide grapheme
					stats.CellsUpdated++
				}
				continue
			}

			if !opts.Force && curRow[x] == cell {
				if runLength > 0 { // the run is broken by an unchanged cell
					closeLink()
					w.WriteString(ansi.Reset)
					runLength = 0
				}
				continue
			}

			begin()

			sameStyle := runLength > 0 && style.FG == cell.FG && style.BG == cell.BG && style.Attr == cell.Attr
			if !sameStyle {
				if runLength > 0 {
					w.WriteString(ansi.Reset)
				}
				runLength = 0
				style = cell
				ansi.MoveTo(w, x+1, y+1) // terminal coordinates are 1-based
				opts.colors.emit(w, cell.FG, false)
				opts.colors.emit(w, cell.BG, true)
				attr := cell.Attr
				if opts.colors.highlight(cell) {
					attr ^= AttrInverse
				}
				emitAttr(w, attr)
			}

			if links {
				if url := cell.Link(); url != link {
					closeLink()
					if url != "" {
						ansi.LinkStart(w, opts.colors.linkID(url), url)
						link = url
					}
				}
			}
			if g := cell.cluster(); g != "" {
				w.WriteString(g)
			} else {
				writeRune(w, cell.Char)
			}
			runLength++
			curRow[x] = cell // the terminal now shows this cell
			stats.CellsUpdated++
		}

		closeLink()
		// A new run only turns attributes on, never off, so close an
		// attributed run before the next row starts.
		if runLength > 0 && (style.Attr != AttrNone || opts.colors.highlight(style)) {
			w.WriteString(ansi.Reset)
		}
	}

	if stats.Started {
		w.WriteString(ansi.Reset)
		if opts.Sync {
			w.WriteString(ansi.SyncEnd)
		}
	}
	return stats
}

func rowsEqual(a, b []Cell) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeRune(w ansi.Writer, r rune) {
	if r >= 0x20 && r < 0x7f { // fast path: printable ASCII
		w.WriteByte(byte(r))
		return
	}
	if r == clearChar || r < 0x20 || r == 0x7f {
		r = ' '
	}
	w.WriteRune(r) // no scratch slice: it would escape through the interface
}

var attrCodes = [...]struct {
	bit  Attr
	code string
}{
	{AttrBold, ansi.Bold}, {AttrDim, ansi.Dim}, {AttrItalic, ansi.Italic},
	{AttrUnderline, ansi.Underline}, {AttrBlink, ansi.Blink}, {AttrInverse, ansi.Inverse},
	{AttrHidden, ansi.Hidden}, {AttrStrikethrough, ansi.Strikethrough},
}

func emitAttr(w ansi.Writer, a Attr) {
	for _, ac := range attrCodes {
		if a&ac.bit != 0 {
			w.WriteString(ac.code)
		}
	}
}
