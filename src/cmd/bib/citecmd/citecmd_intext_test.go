package citecmd

import (
	"bibliography/src/internal/schema"
	"testing"
)

func TestToInTextCitation(t *testing.T) {
	y := 2020
	e := schema.Entry{Type: "article", APA7: schema.APA7{Title: "T", Year: &y, Authors: schema.Authors{{Family: "Doe", Given: "Jane"}}}}
	if s := toInTextCitation(e); s != "(Doe, 2020)" {
		t.Fatalf("unexpected in-text: %q", s)
	}
	// multiple authors -> et al.
	e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: "Roe"}, schema.Author{Family: "Poe"})
	if s := toInTextCitation(e); s != "(Doe et al., 2020)" {
		t.Fatalf("unexpected et al: %q", s)
	}
	// no authors -> fallback to publisher/container/title
	e.APA7.Authors = nil
	e.APA7.Publisher = "Org"
	e.APA7.ContainerTitle = "C"
	e.APA7.Title = "T"
	e.APA7.Year = nil
	e.APA7.Date = ""
	if s := toInTextCitation(e); s == "" {
		t.Fatalf("expected fallback in-text")
	}
}
