package schema

import "testing"

func TestValidateFailuresAndSuccess(t *testing.T) {
	// Missing ID should fail because not UUIDv4 (empty string)
	e := Entry{ID: "", Type: "website", APA7: APA7{Title: "T", URL: "https://e", Accessed: "2025-01-01"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected validation error for empty id")
	}

	// Invalid type
	e = Entry{ID: NewID(), Type: "badtype", APA7: APA7{Title: "T"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected error for invalid type")
	}

	// Missing title
	e = Entry{ID: NewID(), Type: "book", APA7: APA7{Title: ""}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected error for missing title")
	}

	// Missing summary
	e = Entry{ID: NewID(), Type: "book", APA7: APA7{Title: "X"}, Annotation: Annotation{Summary: "", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected error for missing summary")
	}

	// Missing keywords
	e = Entry{ID: NewID(), Type: "book", APA7: APA7{Title: "X"}, Annotation: Annotation{Summary: "s", Keywords: nil}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected error for missing keywords")
	}

	// URL requires accessed
	e = Entry{ID: NewID(), Type: "website", APA7: APA7{Title: "X", URL: "https://x"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected error for missing accessed when url present")
	}

	// Success case
	e = Entry{ID: NewID(), Type: "website", APA7: APA7{Title: "X", URL: "https://x", Accessed: "2025-01-01"}, Annotation: Annotation{Summary: "s", Keywords: []string{"k"}}}
	if err := e.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestSlugifyAndNewID(t *testing.T) {
	y := 2020
	if got := Slugify(" Hello, World!  ", &y); got != "hello-world-2020" {
		t.Fatalf("slug with year: %q", got)
	}
	if got := Slugify("{A}  B  C", nil); got != "a-b-c" {
		t.Fatalf("slug: %q", got)
	}
	id := NewID()
	if !isUUIDv4(id) {
		t.Fatalf("NewID not uuidv4: %q", id)
	}
}
