package schema

import "testing"

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
	e := Entry{ID: "id", Type: "website", APA7: APA7{Title: "Title", URL: "https://x"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected error due to missing accessed when url present")
	}
	e.APA7.Accessed = "2025-01-01"
	if err := e.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
