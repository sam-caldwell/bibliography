package citecmd

import (
	"strings"
	"testing"

	"bibliography/src/internal/schema"
)

func TestAPACitation_DOIvsURL(t *testing.T) {
	e := schema.Entry{Type: "article", APA7: schema.APA7{Title: "T", DOI: "10.1/x"}}
	if s := APACitation(e); !strings.Contains(s, "https://doi.org/10.1/x") {
		t.Fatalf("expected doi link in %q", s)
	}
	e.APA7.DOI = ""
	e.APA7.URL = "https://example.com/x"
	if s := APACitation(e); !strings.Contains(s, "https://example.com/x") {
		t.Fatalf("expected url in %q", s)
	}
}

func TestFormatAuthorVariantsAndYear(t *testing.T) {
	// Author with given initials
	a := formatAuthor(schema.Author{Family: "Doe", Given: "Jane"})
	if !strings.Contains(a, "Doe, J") {
		t.Fatalf("unexpected author format: %q", a)
	}
	// No family -> return given
	if s := formatAuthor(schema.Author{Given: "Org"}); s != "Org" {
		t.Fatalf("given-only should return given: %q", s)
	}
	// Year from Date
	if y := apaYear(schema.Entry{APA7: schema.APA7{Date: "2019-01-02"}}); y != "2019" {
		t.Fatalf("apaYear from date failed: %q", y)
	}
}
