package opentui

import (
	"bytes"
	"strings"
	"testing"
)

var red = RGB(250, 10, 10)

func TestStyledSpansPaintPerSpan(t *testing.T) {
	n := Styled(TextOptions{}, S("ab").Bold(), S("cd").Fg(red), S("ef").Bg(RGB(0, 0, 100)).Italic())
	b := draw(NewNode(Style{AlignItems: AlignStart}, n), 8, 1)
	if got := n.TextContent(); got != "abcdef" {
		t.Fatalf("TextContent = %q", got)
	}
	if c, _ := b.Get(1, 0); c.Char != 'b' || c.Attr != AttrBold || c.FG != White {
		t.Fatalf("bold cell: %+v", c)
	}
	if c, _ := b.Get(2, 0); c.Char != 'c' || c.Attr != AttrNone || c.FG != red {
		t.Fatalf("red cell: %+v", c)
	}
	if c, _ := b.Get(4, 0); c.BG != RGB(0, 0, 100) || c.Attr != AttrItalic {
		t.Fatalf("background span: %+v", c)
	}
	if r := n.Rect(); r.W != 6 || r.H != 1 {
		t.Fatalf("measured %dx%d", r.W, r.H)
	}
}

func TestStyledSpansAcrossLinesAndWraps(t *testing.T) {
	n := Styled(TextOptions{Wrap: WrapWord}, S("hello "), S("world\nnext").Bold(), S(" done").Fg(red))
	root := NewNode(Style{Width: Cells(6), AlignItems: AlignStart}, n)
	b := draw(root, 8, 4)
	want := []string{"hello ", "world ", "next  ", "done  "}
	for y, line := range want {
		if got := strings.TrimRight(strings.Split(screen(b), "\n")[y], " "); got != strings.TrimRight(line, " ") {
			t.Fatalf("row %d = %q, want %q", y, got, line)
		}
	}
	if c, _ := b.Get(0, 1); c.Attr != AttrBold { // "world" is bold on its wrapped row
		t.Fatalf("wrapped bold span lost: %+v", c)
	}
	if c, _ := b.Get(0, 2); c.Attr != AttrBold { // "next" keeps the span after the newline
		t.Fatalf("span across newline lost: %+v", c)
	}
	if c, _ := b.Get(0, 3); c.FG != red || c.Attr != AttrNone {
		t.Fatalf("last span: %+v", c)
	}
}

func TestStyledSpanBoundaryInsideClusterTakesFirstStyle(t *testing.T) {
	n := Styled(TextOptions{}, S("e"), S("́x").Fg(red))
	b := draw(NewNode(Style{AlignItems: AlignStart}, n), 4, 1)
	if c, _ := b.Get(0, 0); c.Grapheme() != "é" || c.FG != White {
		t.Fatalf("cluster split across spans: %+v", c)
	}
	if c, _ := b.Get(1, 0); c.Char != 'x' || c.FG != red {
		t.Fatalf("cell after the cluster: %+v", c)
	}
}

func TestStyledSelectionKeepsAttributesAndLinks(t *testing.T) {
	n := Styled(TextOptions{}, S("ab").Bold(), Link("https://example.com", "cd"))
	r := testRenderer(NewNode(Style{AlignItems: AlignStart}, n))
	r.drawFrame(NewBuffer(6, 1))
	mouse(r, MouseDown, 1, 0)
	mouse(r, MouseDrag, 2, 0)
	if got := r.SelectedText(); got != "bc" {
		t.Fatalf("selected %q", got)
	}
	b := NewBuffer(6, 1)
	r.drawFrame(b)
	if c, _ := b.Get(1, 0); c.BG != n.text.opts.SelectionBG || c.Attr != AttrBold {
		t.Fatalf("selected bold cell: %+v", c)
	}
	if c, _ := b.Get(2, 0); c.BG != n.text.opts.SelectionBG || c.Link() != "https://example.com" {
		t.Fatalf("selected link cell: %+v", c)
	}
	if c, _ := b.Get(3, 0); c.Link() != "https://example.com" || c.BG == n.text.opts.SelectionBG {
		t.Fatalf("unselected link cell: %+v", c)
	}
}

func TestSetTextClearsSpans(t *testing.T) {
	n := Styled(TextOptions{}, S("ab").Bold())
	n.SetText("cd")
	b := draw(NewNode(Style{AlignItems: AlignStart}, n), 4, 1)
	if c, _ := b.Get(0, 0); c.Attr != AttrNone {
		t.Fatalf("SetText kept the old style: %+v", c)
	}
	if spans := n.Spans(); len(spans) != 1 || spans[0] != (Span{Text: "cd"}) {
		t.Fatalf("Spans after SetText = %+v", spans)
	}
}

func encodeLinks(enabled bool, cells ...Cell) string {
	current, next := newPair(len(cells), 1)
	for x, c := range cells {
		next.Set(x, 0, c)
	}
	var out bytes.Buffer
	e := newColorEncoder(ColorTrueColor, Transparent)
	e.links = enabled
	encodeFrame(&out, current, next, FrameOptions{colors: e})
	return out.String()
}

func TestHyperlinkEncoding(t *testing.T) {
	url := "https://example.com/a?b=c"
	link := Cell{Char: 'g', FG: White, BG: Transparent}.WithLink(url)
	plain := Cell{Char: 'p', FG: White, BG: Transparent}
	got := encodeLinks(true, link, Cell{Char: 'o', FG: White, BG: Transparent}.WithLink(url), plain)
	open, closeLink := "\x1b]8;id=1;"+url+"\x1b\\", "\x1b]8;;\x1b\\"
	if !strings.Contains(got, open+"go"+closeLink) {
		t.Fatalf("link not wrapped around its cells: %q", got)
	}
	if strings.Count(got, open) != 1 || !strings.HasSuffix(strings.TrimSuffix(got, "\x1b[0m"), "p") {
		t.Fatalf("link opened more than once or plain cell linked: %q", got)
	}
	if got := encodeLinks(false, link, plain); strings.Contains(got, "]8;") {
		t.Fatalf("hyperlinks disabled but sent: %q", got)
	}
	// A link run broken by an unchanged cell is closed before the cursor moves.
	current, next := newPair(3, 1)
	current.Set(1, 0, plain)
	next.Set(0, 0, link)
	next.Set(1, 0, plain)
	next.Set(2, 0, link)
	var out bytes.Buffer
	e := newColorEncoder(ColorTrueColor, Transparent)
	e.links = true
	encodeFrame(&out, current, next, FrameOptions{colors: e})
	if s := out.String(); strings.Count(s, open) != 2 || strings.Count(s, closeLink) != 2 {
		t.Fatalf("broken link run: %q", s)
	}
}

func TestLinkURLIsSanitized(t *testing.T) {
	n := Styled(TextOptions{}, Link("https://x\x1b]0;pwned\x07/y", "z"))
	b := draw(NewNode(Style{AlignItems: AlignStart}, n), 2, 1)
	c, _ := b.Get(0, 0)
	if strings.ContainsAny(c.Link(), "\x1b\x07") || c.Link() != "https://x]0;pwned/y" {
		t.Fatalf("control bytes kept in link: %q", c.Link())
	}
}

func TestDetectHyperlinks(t *testing.T) {
	for _, tc := range []struct {
		vars map[string]string
		want bool
	}{
		{map[string]string{"TERM": "xterm-kitty"}, true},
		{map[string]string{"TERM": "xterm-ghostty"}, true},
		{map[string]string{"TERM": "alacritty"}, true},
		{map[string]string{"TERM": "xterm-256color"}, false},
		{map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "vscode"}, true},
		{map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app"}, true}, // port: TERM_PROGRAM names
		{map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "Apple_Terminal"}, false},
		{map[string]string{"TERM": "xterm-256color", "VTE_VERSION": "6003"}, true},
		{map[string]string{"TERM": "xterm-256color", "VTE_VERSION": "5000"}, false}, // older VTE can crash on OSC 8
		{map[string]string{"TERM": "tmux-256color", "TERM_PROGRAM": "vscode", "TMUX": "/tmp/t"}, false},
		{map[string]string{"TERM": "xterm-256color", "WSL_DISTRO_NAME": "Ubuntu", "WT_SESSION": "x"}, true},
		{map[string]string{"TERM": "xterm-256color", "FORCE_HYPERLINK": "1"}, true},
		{map[string]string{"TERM": "xterm-kitty", "FORCE_HYPERLINK": "0"}, false},
		{map[string]string{"TERM": "xterm-kitty", "FORCE_HYPERLINK": "off"}, false},
	} {
		if got := DetectHyperlinks(env(tc.vars)); got != tc.want {
			t.Errorf("%v: got %v, want %v", tc.vars, got, tc.want)
		}
	}
}
