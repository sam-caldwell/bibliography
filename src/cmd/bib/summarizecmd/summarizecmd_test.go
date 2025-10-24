package summarizecmd

import (
	"testing"

	"bibliography/src/internal/schema"
)

func TestNeedsSummary(t *testing.T) {
	if !needsSummary(schema.Entry{Annotation: schema.Annotation{Summary: ""}}) {
		t.Fatalf("empty summary should need summary")
	}
	if !needsSummary(schema.Entry{Annotation: schema.Annotation{Summary: "Bibliographic record from X"}}) {
		t.Fatalf("placeholder summary should need summary")
	}
	if needsSummary(schema.Entry{Annotation: schema.Annotation{Summary: "Actual content"}}) {
		t.Fatalf("non-empty real summary should not need summary")
	}
}

func TestWrapText(t *testing.T) {
	s := "one two three four five six seven"
	out := wrapText(s, 10)
	// Expect multiple lines
	if out == s || len(out) == 0 || out == "" {
		t.Fatalf("wrapText produced no wrapping: %q", out)
	}
}

func TestMergeSortDedupKeywords(t *testing.T) {
	out := mergeSortDedupKeywords([]string{"Go", "go"}, []string{"YAML", "json"}, "Article")
	// expect dedup (go once), lowercased and sorted
	if len(out) != 4 || out[0] != "article" || out[1] != "go" || out[2] != "json" || out[3] != "yaml" {
		t.Fatalf("keywords unexpected: %+v", out)
	}
}
