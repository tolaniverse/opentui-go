package opentui

import "testing"

func TestInputGraphemeNavigationAndDeletion(t *testing.T) {
	n := Input(InputOptions{Value: "Ae\u0301界👩🏽‍💻Z"})
	if n.Cursor() != len([]rune(n.Value())) {
		t.Fatal("cursor must start at the end in rune offsets")
	}
	n.keyHandler(press("left", "\x1b[D")) // Z
	n.keyHandler(press("left", "\x1b[D")) // emoji cluster
	if n.Cursor() != 4 {                  // A e accent 界
		t.Fatalf("left crossed inside an emoji cluster: cursor %d, want 4", n.Cursor())
	}
	n.keyHandler(press("backspace", "\x7f"))
	if got := n.Value(); got != "Ae\u0301👩🏽‍💻Z" || n.Cursor() != 3 {
		t.Fatalf("backspace of wide glyph: %q at %d", got, n.Cursor())
	}
	n.keyHandler(press("backspace", "\x7f"))
	if got := n.Value(); got != "A👩🏽‍💻Z" || n.Cursor() != 1 {
		t.Fatalf("backspace of combining cluster: %q at %d", got, n.Cursor())
	}
	n.keyHandler(press("delete", "\x1b[3~"))
	if got := n.Value(); got != "AZ" || n.Cursor() != 1 {
		t.Fatalf("delete of emoji cluster: %q at %d", got, n.Cursor())
	}
}

func TestInputCursorSnapsToClusterBoundary(t *testing.T) {
	n := Input(InputOptions{Value: "Ae\u0301B"})
	n.SetCursor(2) // between e and its accent is not a valid editing point
	if got := n.Cursor(); got != 1 {
		t.Fatalf("cursor inside cluster = %d, want 1", got)
	}
	n.InsertText("X")
	if got := n.Value(); got != "AXe\u0301B" {
		t.Fatalf("insertion split a cluster: %q", got)
	}
}

func TestInputPaintUsesCellWidths(t *testing.T) {
	n := Input(InputOptions{Value: "A界B", Style: Style{Width: Cells(4)}})
	root := NewNode(Style{AlignItems: AlignStart}, n)
	r := testRenderer(root)
	r.Focus(n)
	n.SetCursor(2) // before B, which is at terminal column 3
	b := draw(root, 4, 1)
	if c, _ := b.Get(3, 0); c.Char != 'B' || c.Attr&AttrInverse == 0 {
		t.Fatalf("cursor before B not at column 3: %+v", c)
	}
	n.SetCursor(3) // after B; need a free cell for the cursor
	b = draw(root, 4, 1)
	if c, _ := b.Get(3, 0); c.Char != ' ' || c.Attr&AttrInverse == 0 {
		t.Fatalf("cursor at end not visible at column 3: %+v", c)
	}
	if c, _ := b.Get(0, 0); c.Char != '界' {
		t.Fatalf("scrolling split wide glyph: %+v", c)
	}
}

func TestInputMaxLengthDoesNotSplitCluster(t *testing.T) {
	n := Input(InputOptions{MaxLength: 2, Value: "A👩🏽‍💻B"})
	if got := n.Value(); got != "A👩🏽‍💻" {
		t.Fatalf("max length split a grapheme: %q", got)
	}
	if n.InsertText("Z") {
		t.Fatal("inserted beyond two grapheme clusters")
	}
}

func TestInputLengthCountsClustersAndCombiningInsert(t *testing.T) {
	n := Input(InputOptions{MaxLength: 1, MinLength: 1})
	if n.Submit() {
		t.Fatal("empty input met MinLength")
	}
	if !n.InsertText("e") || !n.InsertText("\u0301") {
		t.Fatal("combining mark should merge without consuming a second slot")
	}
	if got := n.Value(); got != "e\u0301" || n.Cursor() != 2 || !n.Submit() {
		t.Fatalf("merged input: %q cursor %d submit %v", got, n.Cursor(), n.Submit())
	}
	if n.InsertText("X") {
		t.Fatal("second cluster exceeded MaxLength")
	}
}
