package store

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"bibliography/src/internal/schema"
)

func TestUpdateVerifyListAndFormatBib(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Create two entries and upsert
	y := 2021
	e1 := schema.Entry{ID: schema.NewID(), Type: "article", APA7: schema.APA7{Title: "Alpha", Year: &y, Journal: "J", Pages: "1-2"}, Annotation: schema.Annotation{Summary: strings.Repeat("a ", 50), Keywords: []string{"k1"}}}
	e2 := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "Beta", URL: "https://ex", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "sum", Keywords: []string{"k2"}}}

	if _, err := WriteEntry(e1); err != nil {
		t.Fatalf("write e1: %v", err)
	}
	if _, err := WriteEntry(e2); err != nil {
		t.Fatalf("write e2: %v", err)
	}
	// Library written
	b, err := os.ReadFile(BibFile)
	if err != nil || len(b) == 0 {
		t.Fatalf("missing bibfile: %v", err)
	}
	// Verify metadata presence for new records
	if !strings.Contains(string(b), "created = {") || !strings.Contains(string(b), "verified = {false}") || !strings.Contains(string(b), "verified_by = {}") {
		t.Fatalf("expected metadata fields in bib: %s", string(b))
	}

	// Verify one entry by id
	if err := VerifyByID(e1.ID, "tester"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	b, _ = os.ReadFile(BibFile)
	if !strings.Contains(string(b), "_id = {"+e1.ID+"}") || !strings.Contains(string(b), "verified = {true}") || !strings.Contains(string(b), "verified_by = {tester}") {
		t.Fatalf("expected verified fields set for e1: %s", string(b))
	}

	// List unverified should return only e2
	pending, err := ListUnverified()
	if err != nil {
		t.Fatalf("list unverified: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != e2.ID {
		t.Fatalf("unexpected pending: %+v", pending)
	}

	// Update source by id
	if err := UpdateSourceByID(e2.ID, "manual-test"); err != nil {
		t.Fatalf("update source: %v", err)
	}
	b, _ = os.ReadFile(BibFile)
	if !strings.Contains(string(b), "_id = {"+e2.ID+"}") || !strings.Contains(string(b), "source = {manual-test}") {
		t.Fatalf("expected updated source: %s", string(b))
	}

	// Format with a narrow width to exercise wrapping path
	if err := FormatBibLibrary(60); err != nil {
		t.Fatalf("format: %v", err)
	}
	fb, _ := os.ReadFile(BibFile)
	if !strings.Contains(string(fb), "  abstract = {") {
		t.Fatalf("expected abstract field present after format")
	}
}

func TestHelpersAuthorsKeywordsAndWrap(t *testing.T) {
	// parseAuthorsField
	as := parseAuthorsField("Doe, J and Smith, A and Acme Corp")
	if len(as) != 3 || as[0].Family != "Doe" || as[1].Given != "A" || as[2].Family != "Acme Corp" {
		t.Fatalf("parseAuthorsField unexpected: %+v", as)
	}
	// splitKeywords dedup + sort
	ks := splitKeywords("Go, go, YAML , json")
	if len(ks) != 3 || ks[0] != "go" || ks[1] != "json" || ks[2] != "yaml" {
		t.Fatalf("splitKeywords: %+v", ks)
	}
	// writeWrappedField should wrap long content
	var buf bytes.Buffer
	writeWrappedField(&buf, "abstract", strings.Repeat("word ", 30), 50)
	out := buf.String()
	if !strings.Contains(out, "\n    ") { // continuation indent
		t.Fatalf("expected wrapped continuation, got: %q", out)
	}
}

func TestParseBibAndRoundTrip(t *testing.T) {
	// Build a small bib with two records and comments
	bib := `
% comment line
@article{a1,
  author = {Doe, J and Smith, A},
  title = {Alpha Beta},
  journal = {J},
  year = {2020},
  abstract = {Summary S},
  keywords = {a, b, a},
  _id = {11111111-1111-4111-8111-111111111111},
  _type = {article},
  created = {2025-01-01T00:00:00Z},
  modified = {2025-01-01T00:00:00Z},
  source = {manual},
  verified = {false},
  verified_by = {}
}

@book{b1,
  author = {Acme Corp},
  title = {Book Title},
  publisher = {P},
  year = {1999},
  _id = {22222222-2222-4222-8222-222222222222},
  _type = {book},
  verified = {true},
  verified_by = {tester}
}
`
	recs, err := parseBib(bib)
	if err != nil || len(recs) != 2 {
		t.Fatalf("parseBib err=%v len=%d", err, len(recs))
	}
	// Convert to entries and validate fields
	es := bibToEntries(recs)
	if len(es) != 2 {
		t.Fatalf("entries len: %d", len(es))
	}
	// Check authors parsing and keywords normalization
	if len(es[0].APA7.Authors) != 2 || es[0].Annotation.Summary == "" || len(es[0].Annotation.Keywords) != 2 {
		t.Fatalf("unexpected entry0: %+v", es[0])
	}
	if es[1].Type != "book" || es[1].APA7.Publisher != "P" {
		t.Fatalf("unexpected entry1: %+v", es[1])
	}
}

func TestSetWriteSourceAndReplace(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	e := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "T", Publisher: "P"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	SetWriteSource("web")
	if _, err := WriteEntry(e); err != nil {
		t.Fatalf("write: %v", err)
	}
	b1, _ := os.ReadFile(BibFile)
	if !strings.Contains(string(b1), "source = {web}") {
		t.Fatalf("expected source=web: %s", string(b1))
	}
	// Replace same id with different title; ensure only one record and modified updates
	e.APA7.Title = "T2"
	SetWriteSource("manual")
	if _, err := WriteEntry(e); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	b2, _ := os.ReadFile(BibFile)
	if strings.Count(string(b2), "@") != 1 || !strings.Contains(string(b2), "title = {T2}") {
		t.Fatalf("expected single updated record: %s", string(b2))
	}
}
