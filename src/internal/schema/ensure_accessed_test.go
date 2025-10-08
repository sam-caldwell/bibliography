package schema

import "testing"

func TestEnsureAccessedIfURL(t *testing.T) {
	e := Entry{ID: NewID(), Type: "website", APA7: APA7{Title: "T", URL: "https://example"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if e.APA7.Accessed != "" {
		t.Fatalf("precondition unexpected accessed")
	}
	EnsureAccessedIfURL(&e)
	if e.APA7.Accessed == "" {
		t.Fatalf("expected accessed to be set")
	}
}
