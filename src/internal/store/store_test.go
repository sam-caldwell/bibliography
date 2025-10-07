package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bibliography/src/internal/schema"
)

func TestWriteReadAndIndex(t *testing.T) {
	dir := t.TempDir()
	// Change into temp
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	e1 := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "A", URL: "https://a", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s1", Keywords: []string{"Go", "APA7"}}}
	e2 := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "B", ISBN: "123-456", DOI: "10.1/abc"}, Annotation: schema.Annotation{Summary: "s2", Keywords: []string{"go", "yaml"}}}

	p1, err := WriteEntry(e1)
	if err != nil {
		t.Fatalf("write1: %v", err)
	}
	if _, err := os.Stat(p1); err != nil {
		t.Fatalf("stat1: %v", err)
	}
	p2, err := WriteEntry(e2)
	if err != nil {
		t.Fatalf("write2: %v", err)
	}
	if _, err := os.Stat(p2); err != nil {
		t.Fatalf("stat2: %v", err)
	}

	list, err := ReadAll()
	if err != nil {
		t.Fatalf("readall: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 entries got %d", len(list))
	}

	out, err := BuildKeywordIndex(list)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if out != KeywordsJSON {
		t.Fatalf("unexpected path: %s", out)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("index stat: %v", err)
	}

	// Also build authors, titles, ISBN, and DOI index
	aout, err := BuildAuthorIndex(list)
	if err != nil {
		t.Fatalf("author index: %v", err)
	}
	if aout != AuthorsJSON {
		t.Fatalf("unexpected authors path: %s", aout)
	}
	if _, err := os.Stat(aout); err != nil {
		t.Fatalf("authors index stat: %v", err)
	}
	tout, err := BuildTitleIndex(list)
	if err != nil {
		t.Fatalf("title index: %v", err)
	}
	if tout != TitlesJSON {
		t.Fatalf("unexpected titles path: %s", tout)
	}
	if _, err := os.Stat(tout); err != nil {
		t.Fatalf("titles index stat: %v", err)
	}
	iout, err := BuildISBNIndex(list)
	if err != nil {
		t.Fatalf("isbn index: %v", err)
	}
	if iout != ISBNJSON {
		t.Fatalf("unexpected isbn path: %s", iout)
	}
	if _, err := os.Stat(iout); err != nil {
		t.Fatalf("isbn index stat: %v", err)
	}
	dout, err := BuildDOIIndex(list)
	if err != nil {
		t.Fatalf("doi index: %v", err)
	}
	if dout != DOIJSON {
		t.Fatalf("unexpected doi path: %s", dout)
	}
	if _, err := os.Stat(dout); err != nil {
		t.Fatalf("doi index stat: %v", err)
	}

	// Filter AND semantics: Go + APA7 should match only e1
	matches := FilterByKeywordsAND(list, []string{"go", "apa7"})
	if len(matches) != 1 || matches[0].ID != e1.ID {
		t.Fatalf("filter AND mismatch: %+v", matches)
	}
}

func TestEmptyDirs(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	entries, err := ReadAll()
	if err != nil {
		t.Fatalf("readall: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries")
	}
	// Ensure metadata dir creation on index
	if _, err := BuildKeywordIndex(entries); err != nil {
		t.Fatalf("index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(MetadataDir)); err != nil {
		t.Fatalf("metadata dir: %v", err)
	}
}

func TestIndexIncludesRequestedFields(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	y1 := 1984
	e1 := schema.Entry{
		ID:   schema.NewID(),
		Type: "article",
		APA7: schema.APA7{
			Title:     "End to End Arguments in System Design",
			Year:      &y1,
			Journal:   "ACM",
			Publisher: "ACM",
			URL:       "https://doi.org/10.1145/1",
			Accessed:  "2025-01-01",
			Authors:   schema.Authors{{Family: "Doe", Given: "J."}},
			DOI:       "10.1145/1",
		},
		Annotation: schema.Annotation{Summary: "s", Keywords: []string{"Networks"}},
	}

	y2 := 2024
	e2 := schema.Entry{
		ID:   schema.NewID(),
		Type: "website",
		APA7: schema.APA7{
			Title:     "JSON at Example",
			Year:      &y2,
			Publisher: "Example Org",
			URL:       "https://www.example.com/a/b",
			Accessed:  "2025-01-01",
			Authors:   schema.Authors{{Family: "National Automated Clearing House Association"}},
		},
		Annotation: schema.Annotation{Summary: "Web tools and tutorials", Keywords: []string{"web"}},
	}

	if _, err := WriteEntry(e1); err != nil {
		t.Fatalf("write e1: %v", err)
	}
	if _, err := WriteEntry(e2); err != nil {
		t.Fatalf("write e2: %v", err)
	}

	list, err := ReadAll()
	if err != nil {
		t.Fatalf("readall: %v", err)
	}
	if _, err := BuildKeywordIndex(list); err != nil {
		t.Fatalf("index: %v", err)
	}
	if _, err := BuildAuthorIndex(list); err != nil {
		t.Fatalf("authors index: %v", err)
	}
	if _, err := BuildTitleIndex(list); err != nil {
		t.Fatalf("titles index: %v", err)
	}
	if _, err := BuildISBNIndex(list); err != nil { // no books here; should still succeed
		t.Fatalf("isbn index: %v", err)
	}
	if _, err := BuildDOIIndex(list); err != nil {
		t.Fatalf("doi index: %v", err)
	}
	// verify authors index content
	araw, err := os.ReadFile(AuthorsJSON)
	if err != nil {
		t.Fatalf("read authors index: %v", err)
	}
	var aidx map[string][]string
	if err := json.Unmarshal(araw, &aidx); err != nil {
		t.Fatalf("unmarshal authors index: %v", err)
	}
	if !contains(aidx["Doe, J."], "data/citations/article/"+e1.ID+".yaml") {
		t.Fatalf("missing author work for Doe, J.: %+v", aidx)
	}
	if !contains(aidx["National Automated Clearing House Association"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing corporate author work: %+v", aidx)
	}

	// verify titles index content
	traw, err := os.ReadFile(TitlesJSON)
	if err != nil {
		t.Fatalf("read titles index: %v", err)
	}
	var tidx map[string][]string
	if err := json.Unmarshal(traw, &tidx); err != nil {
		t.Fatalf("unmarshal titles index: %v", err)
	}
	// titles index now stores tokenized title words
	if ws := tidx["data/citations/article/"+e1.ID+".yaml"]; !containsStr(ws, "system") || !containsStr(ws, "design") {
		t.Fatalf("missing expected title tokens for art1: %+v", ws)
	}
	if ws := tidx["data/citations/site/"+e2.ID+".yaml"]; !containsStr(ws, "json") || !containsStr(ws, "example") {
		t.Fatalf("missing expected title tokens for site1: %+v", ws)
	}

	// verify isbn index content for this test set is empty
	iraw, err := os.ReadFile(ISBNJSON)
	if err != nil {
		t.Fatalf("read isbn index: %v", err)
	}
	var iidx map[string]string
	if err := json.Unmarshal(iraw, &iidx); err != nil {
		t.Fatalf("unmarshal isbn index: %v", err)
	}
	if len(iidx) != 0 {
		t.Fatalf("expected empty isbn index, got: %+v", iidx)
	}
	// verify doi index contains the article mapping
	draw, err := os.ReadFile(DOIJSON)
	if err != nil {
		t.Fatalf("read doi index: %v", err)
	}
	var didx map[string]string
	if err := json.Unmarshal(draw, &didx); err != nil {
		t.Fatalf("unmarshal doi index: %v", err)
	}
	if got := didx["data/citations/article/"+e1.ID+".yaml"]; got != "10.1145/1" {
		t.Fatalf("wrong or missing doi for art1: %q (idx=%+v)", got, didx)
	}
	raw, err := os.ReadFile(KeywordsJSON)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx map[string][]string
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}

	// keywords map to paths with type segment
	if !contains(idx["networks"], "data/citations/article/"+e1.ID+".yaml") || !contains(idx["web"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing keyword tokens: %+v", idx)
	}
	// title words
	if !contains(idx["system"], "data/citations/article/"+e1.ID+".yaml") || !contains(idx["json"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing title tokens: %+v", idx)
	}
	// publisher
	if !contains(idx["acm"], "data/citations/article/"+e1.ID+".yaml") || !contains(idx["example"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing publisher tokens: %+v", idx)
	}
	// year
	if !contains(idx["1984"], "data/citations/article/"+e1.ID+".yaml") || !contains(idx["2024"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing year tokens: %+v", idx)
	}
	// domain (host and host without www.)
	if !contains(idx["doi.org"], "data/citations/article/"+e1.ID+".yaml") || !contains(idx["www.example.com"], "data/citations/site/"+e2.ID+".yaml") || !contains(idx["example.com"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing domain tokens: %+v", idx)
	}
	// type
	if !contains(idx["article"], "data/citations/article/"+e1.ID+".yaml") || !contains(idx["website"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing type tokens: %+v", idx)
	}
	// summary tokens
	if !contains(idx["tools"], "data/citations/site/"+e2.ID+".yaml") || !contains(idx["tutorials"], "data/citations/site/"+e2.ID+".yaml") {
		t.Fatalf("missing summary tokens: %+v", idx)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func containsStr(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestTokenizeAndDOIAndNormalize(t *testing.T) {
	if got := tokenizeWords("Hello, YAML!"); len(got) != 2 || got[0] != "hello" || got[1] != "yaml" {
		t.Fatalf("tokenizeWords: %+v", got)
	}
	if d := ExtractDOI("https://doi.org/10.1000/ABC.123"); d != "10.1000/ABC.123" {
		t.Fatalf("ExtractDOI: %q", d)
	}
	// NormalizeArticleDOI sets DOI and doi.org URL for article
	y := 2020
	e := schema.Entry{ID: "x", Type: "article", APA7: schema.APA7{Title: "T", Year: &y, URL: "https://publisher.com/doi/10.1234/x", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	if !NormalizeArticleDOI(&e) {
		t.Fatalf("expected normalize to modify entry")
	}
	if e.APA7.DOI == "" || !strings.Contains(e.APA7.URL, "https://doi.org/") {
		t.Fatalf("normalize fields: %+v", e.APA7)
	}
}

// Migration removed; keep segment mapping coverage
func TestSegmentForType(t *testing.T) {
	if SegmentForType("website") != "site" {
		t.Fatalf("website segment")
	}
	if SegmentForType("book") != "books" {
		t.Fatalf("book segment")
	}
	if SegmentForType("article") != "article" {
		t.Fatalf("article segment")
	}
	if SegmentForType("rfc") != "rfc" {
		t.Fatalf("rfc segment")
	}
	if SegmentForType("video") != "video" {
		t.Fatalf("video segment")
	}
	if SegmentForType("unknown") != "citation" {
		t.Fatalf("default segment")
	}
}

func TestFilterByKeywordsAND_AndExtractDOI(t *testing.T) {
	es := []schema.Entry{
		{ID: schema.NewID(), Type: "article", APA7: schema.APA7{Title: "T", DOI: "10.1/x"}, Annotation: schema.Annotation{Keywords: []string{"Go", "YAML"}}},
		{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "B"}, Annotation: schema.Annotation{Keywords: []string{"go"}}},
	}
	out := FilterByKeywordsAND(es, []string{"go", "yaml"})
	if len(out) != 1 || out[0].Type != "article" {
		t.Fatalf("AND filter wrong: %+v", out)
	}
	if d := ExtractDOI("https://doi.org/10.5555/ABC"); d == "" {
		t.Fatalf("expected doi extracted")
	}
}
