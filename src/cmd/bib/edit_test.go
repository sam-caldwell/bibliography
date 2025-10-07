package main

import (
    "testing"
    "gopkg.in/yaml.v3"
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
    if err := setYAMLPathValue(root, "apa7.title", "Hello"); err != nil {
        t.Fatalf("setYAMLPathValue: %v", err)
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

func TestSetYAMLPathValue_SequenceValue(t *testing.T) {
    root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
    // YAML sequence value
    if err := setYAMLPathValue(root, "annotation.keywords", "[a, b]"); err != nil {
        t.Fatalf("setYAMLPathValue: %v", err)
    }
    ann := findMapValue(root, "annotation")
    if ann == nil {
        t.Fatalf("annotation mapping missing")
    }
    kw := findMapValue(ann, "keywords")
    if kw == nil || kw.Kind != yaml.SequenceNode || len(kw.Content) != 2 {
        t.Fatalf("keywords not a sequence of length 2: %+v", kw)
    }
    if kw.Content[0].Value != "a" || kw.Content[1].Value != "b" {
        t.Fatalf("keywords values wrong: %q,%q", kw.Content[0].Value, kw.Content[1].Value)
    }
}

func TestSetYAMLPathValue_MappingValue(t *testing.T) {
    root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
    // sequence of mappings (authors)
    raw := "- family: Doe\n  given: Jane\n- family: Poe\n"
    if err := setYAMLPathValue(root, "apa7.authors", raw); err != nil {
        t.Fatalf("setYAMLPathValue: %v", err)
    }
    apa7 := findMapValue(root, "apa7")
    if apa7 == nil {
        t.Fatalf("apa7 missing")
    }
    authors := findMapValue(apa7, "authors")
    if authors == nil || authors.Kind != yaml.SequenceNode || len(authors.Content) != 2 {
        t.Fatalf("authors not parsed: %+v", authors)
    }
    if findMapValue(authors.Content[0], "family").Value != "Doe" {
        t.Fatalf("first author family wrong")
    }
}

func TestSetYAMLPathValue_OverwriteValue(t *testing.T) {
    root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
    if err := setYAMLPathValue(root, "apa7.title", "Old"); err != nil { t.Fatal(err) }
    if err := setYAMLPathValue(root, "apa7.title", "New"); err != nil { t.Fatal(err) }
    apa7 := findMapValue(root, "apa7")
    title := findMapValue(apa7, "title")
    if title.Value != "New" {
        t.Fatalf("expected overwrite to New, got %q", title.Value)
    }
}

func TestSetYAMLPathValue_TypeChangeOnLeaf(t *testing.T) {
    root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
    // Create a mapping at apa7
    if err := setYAMLPathValue(root, "apa7.title", "X"); err != nil { t.Fatal(err) }
    // Replace apa7 mapping with a scalar
    if err := setYAMLPathValue(root, "apa7", "scalar"); err != nil { t.Fatal(err) }
    apa7 := findMapValue(root, "apa7")
    if apa7 == nil || apa7.Kind != yaml.ScalarNode || apa7.Value != "scalar" {
        t.Fatalf("apa7 not replaced by scalar: %+v", apa7)
    }
}

func TestSplitDotPath_EmptySegmentError(t *testing.T) {
    if _, err := splitDotPath("apa7..title"); err == nil {
        t.Fatalf("expected error for empty segment")
    }
}

func TestSetYAMLPathValue_FallbackToStringOnInvalidYAML(t *testing.T) {
    root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
    raw := ":\n" // invalid YAML
    if err := setYAMLPathValue(root, "note", raw); err != nil { t.Fatal(err) }
    note := findMapValue(root, "note")
    if note == nil || note.Kind != yaml.ScalarNode || note.Value != raw {
        t.Fatalf("invalid raw should set scalar with raw value, got: %+v", note)
    }
}

