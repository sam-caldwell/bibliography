package schema

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSlugify(t *testing.T) {
	y := 2020
	cases := []struct {
		in   string
		year *int
		want string
	}{
		{"Hello, World!", nil, "hello-world"},
		{" Go  &  YAML ", &y, "go-yaml-2020"},
		{"  multiple---dashes__here ", nil, "multiple-dashes-here"},
	}
	for _, c := range cases {
		got := Slugify(c.in, c.year)
		if got != c.want {
			t.Fatalf("Slugify(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	e := Entry{ID: NewID(), Type: "website", APA7: APA7{Title: "Title", URL: "https://x"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected error due to missing accessed when url present")
	}
	e.APA7.Accessed = "2025-01-01"
	if err := e.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	e := Entry{ID: "id", Type: "unknown", APA7: APA7{Title: "T"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected invalid type error")
	}
	e = Entry{ID: "id", Type: "book", APA7: APA7{}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected missing title error")
	}
	e = Entry{ID: "id", Type: "book", APA7: APA7{Title: "T"}, Annotation: Annotation{Summary: "", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected missing summary error")
	}
	e = Entry{ID: "id", Type: "book", APA7: APA7{Title: "T"}, Annotation: Annotation{Summary: "s", Keywords: nil}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected missing keywords error")
	}
}

func TestAuthorsUnmarshalUnknownShape(t *testing.T) {
	// Sequence with empty entries and mapping empty should result in empty Authors
	yml := "authors: [ ]\n"
	var n yaml.Node
	if err := yaml.NewDecoder(strings.NewReader(yml)).Decode(&n); err != nil {
		t.Fatal(err)
	}
	root := n.Content[0]
	var val *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "authors" {
			val = root.Content[i+1]
			break
		}
	}
	var a Authors
	if err := a.UnmarshalYAML(val); err != nil {
		t.Fatal(err)
	}
	if len(a) != 0 {
		t.Fatalf("expected empty authors, got %d", len(a))
	}
}

func TestAuthorsUnmarshalFlexible(t *testing.T) {
	cases := []struct {
		name        string
		yamlStr     string
		wantLen     int
		wantFamily0 string
	}{
		{"scalar_corporate", "authors: National Automated Clearing House Association\n", 1, "National Automated Clearing House Association"},
		{"seq_strings", "authors: [Apple, Google]\n", 2, "Apple"},
		{"mapping_single", "authors:\n  family: Doe\n  given: John\n", 1, "Doe"},
		{"seq_mappings", "authors:\n  - family: Doe\n    given: J.\n  - family: Smith\n", 2, "Doe"},
	}
	for _, c := range cases {
		var a Authors
		var node yaml.Node
		if err := yaml.NewDecoder(strings.NewReader(c.yamlStr)).Decode(&node); err != nil {
			t.Fatalf("%s decode node: %v", c.name, err)
		}
		// Find the mapping of authors directly
		if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
			t.Fatalf("%s bad root", c.name)
		}
		root := node.Content[0]
		if root.Kind != yaml.MappingNode {
			t.Fatalf("%s expected mapping root", c.name)
		}
		// find the value node for key 'authors'
		var val *yaml.Node
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value == "authors" {
				val = root.Content[i+1]
				break
			}
		}
		if val == nil {
			t.Fatalf("%s authors node not found", c.name)
		}
		if err := a.UnmarshalYAML(val); err != nil {
			t.Fatalf("%s unmarshal: %v", c.name, err)
		}
		if len(a) != c.wantLen {
			t.Fatalf("%s len=%d want %d", c.name, len(a), c.wantLen)
		}
		if c.wantLen > 0 && a[0].Family != c.wantFamily0 {
			t.Fatalf("%s family0=%q want %q", c.name, a[0].Family, c.wantFamily0)
		}
	}
}
