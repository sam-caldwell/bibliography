package summarize

import "testing"

func TestExtractJSONObject(t *testing.T) {
	s := "prefix {\"a\":1} suffix"
	got, err := extractJSONObject(s)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}
	if got != "{\"a\":1}" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestNormalizeKeywords(t *testing.T) {
	in := []string{"Alpha", "alpha", " Beta ", "", "Gamma"}
	out := normalizeKeywords(in)
	if len(out) != 3 {
		t.Fatalf("expected 3 unique, got %v", out)
	}
}
