package opentui

import "testing"

func TestTextTruncateKeepsBothEnds(t *testing.T) {
	for _, tc := range []struct {
		width int
		want  string
	}{
		{3, "   "}, {4, "...j"}, {5, "a...j"}, {6, "a...ij"},
		{7, "ab...ij"}, {10, "abcdefghij"},
	} {
		n := Text("abcdefghij", TextOptions{Truncate: true, Style: Style{Width: Cells(tc.width)}})
		root := NewNode(Style{AlignItems: AlignStart}, n)
		b := draw(root, tc.width, 1)
		if got := screen(b); got != tc.want+"\n" {
			t.Errorf("width %d: %q, want %q", tc.width, got, tc.want)
		}
		if n.TextContent() != "abcdefghij" {
			t.Fatal("truncation changed source text")
		}
	}
}

func TestTextTruncateUnicodeAndStyledSpans(t *testing.T) {
	red := RGB(240, 20, 20)
	n := Styled(TextOptions{Truncate: true, Style: Style{Width: Cells(8)}},
		S("界界").Fg(red), S("xxxx界"))
	b := draw(NewNode(Style{}, n), 8, 1)
	if got := rowText(b, 0); got != "界...x界" {
		t.Fatalf("truncated wide glyphs: %q", got)
	}
	if c, _ := b.Get(0, 0); c.FG != red {
		t.Fatalf("prefix span style lost: %+v", c)
	}
}

func TestTextTruncateSelectionUsesSourceOffsets(t *testing.T) {
	n := Text("abcdefghij", TextOptions{Truncate: true, Style: Style{Width: Cells(6)}})
	r := testRenderer(NewNode(Style{}, n))
	r.drawFrame(NewBuffer(6, 1))
	r.startSelection(n.Node, 0, 0)
	r.updateSelection(5, 0)
	if got := r.SelectedText(); got != "abcdefghij" {
		t.Fatalf("selection lost hidden source: %q", got)
	}
	b := NewBuffer(6, 1)
	r.drawFrame(b)
	if got := screen(b); got != "a...ij\n" {
		t.Fatalf("selection overwrote ellipsis: %q", got)
	}
}
