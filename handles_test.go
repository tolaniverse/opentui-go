package opentui

import (
	"reflect"
	"testing"
)

func TestTypedWidgetMethods(t *testing.T) {
	for _, tc := range []struct {
		typeOf reflect.Type
		method string
	}{
		{reflect.TypeOf((*Node)(nil)), "SetValue"},
		{reflect.TypeOf((*Node)(nil)), "SetFilter"},
		{reflect.TypeOf((*TextNode)(nil)), "SetValue"},
		{reflect.TypeOf((*InputNode)(nil)), "SetFilter"},
		{reflect.TypeOf((*TabSelectNode)(nil)), "SetFilter"},
	} {
		if _, exists := tc.typeOf.MethodByName(tc.method); exists {
			t.Fatalf("%v exposes wrong-widget method %s", tc.typeOf, tc.method)
		}
	}
	root := NewNode(Style{})
	text := Text("hello", TextOptions{})
	input := Input(InputOptions{})
	selectNode := Select(SelectOptions{})
	root.Add(text, input, selectNode)
	if got := root.Children(); len(got) != 3 || got[1] != input.Node {
		t.Fatalf("tree did not preserve typed widget identity: %v", got)
	}
	input.SetValue("abc")
	if input.Value() != "abc" || text.TextContent() != "hello" {
		t.Fatal("typed widget methods did not reach the underlying state")
	}
	input.ID = "edit"
	if got := AsInput(root.Find("edit")); got == nil || got.Node != input.Node {
		t.Fatal("Find lost the input handle")
	}
	if AsSelect(input.Node) != nil || AsInput(text.Node) != nil {
		t.Fatal("wrong-widget conversion succeeded")
	}
	var absent *InputNode
	root.Add(absent)
	if root.Remove(absent) || len(root.Children()) != 3 {
		t.Fatal("nil widget handle changed the tree")
	}
}

func TestHandleRecoveryAndTreeAPIs(t *testing.T) {
	text := Styled(TextOptions{}, S("styled"))
	textarea := Textarea(TextareaOptions{})
	list := Select(SelectOptions{})
	tabs := TabSelect(TabSelectOptions{})
	scroll := ScrollBox(ScrollBoxOptions{}, text)
	root := NewNode(Style{}, textarea, list, tabs, scroll)
	for _, tc := range []struct {
		name    string
		node    *Node
		recover func(*Node) NodeHandle
	}{
		{"text", text.Node, func(n *Node) NodeHandle { return AsText(n) }},
		{"textarea", textarea.Node, func(n *Node) NodeHandle { return AsTextarea(n) }},
		{"select", list.Node, func(n *Node) NodeHandle { return AsSelect(n) }},
		{"tabs", tabs.Node, func(n *Node) NodeHandle { return AsTabSelect(n) }},
		{"scroll", scroll.Node, func(n *Node) NodeHandle { return AsScrollBox(n) }},
	} {
		if got := tc.recover(tc.node); got == nil || got.AsNode() != tc.node {
			t.Errorf("%s recovery did not preserve node identity", tc.name)
		}
		if got := tc.recover(nil); got == nil || got.AsNode() != nil {
			t.Errorf("%s recovered a nil node: %v", tc.name, got)
		}
	}
	if AsText(list.Node) != nil || AsTextarea(tabs.Node) != nil || AsTabSelect(scroll.Node) != nil || AsScrollBox(text.Node) != nil {
		t.Fatal("wrong-widget conversion succeeded")
	}
	if !root.Remove(tabs) || tabs.Parent() != nil {
		t.Fatal("typed Remove failed")
	}
	if !scroll.Remove(text) || text.Parent() != nil {
		t.Fatal("typed scroll child Remove failed")
	}
	if r := testRenderer(root); !r.Focus(textarea) || r.Focused() != textarea.Node {
		t.Fatal("typed Focus failed")
	}
	ComputeLayout(root, 20, 5)
	Draw(Text("hello", TextOptions{}), NewBuffer(20, 5))
}
