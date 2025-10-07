package editcmd

import (
	"gopkg.in/yaml.v3"
	"testing"
)

// helper: find mapping value by key
func findMapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func TestSetYAMLPathValue_NestedScalar(t *testing.T) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if err := SetYAMLPathValue(root, "apa7.title", "Hello"); err != nil {
		t.Fatalf("SetYAMLPathValue: %v", err)
	}
	apa7 := findMapValue(root, "apa7")
	if apa7 == nil || apa7.Kind != yaml.MappingNode {
		t.Fatalf("apa7 mapping not created")
	}
	title := findMapValue(apa7, "title")
	if title == nil || title.Kind != yaml.ScalarNode || title.Value != "Hello" {
		t.Fatalf("title not set correctly: %+v", title)
	}
}

func TestSplitDotPath_EmptySegmentError(t *testing.T) {
	if _, err := SplitDotPath("apa7..title"); err == nil {
		t.Fatalf("expected error for empty segment")
	}
}
