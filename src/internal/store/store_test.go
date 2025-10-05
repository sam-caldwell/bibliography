package store

import (
	"os"
	"path/filepath"
	"testing"

	"bibliography/src/internal/schema"
)

func TestWriteReadAndIndex(t *testing.T) {
	dir := t.TempDir()
	// Change into temp
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	e1 := schema.Entry{ID: "a", Type: "website", APA7: schema.APA7{Title: "A", URL: "https://a", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s1", Keywords: []string{"Go", "APA7"}}}
	e2 := schema.Entry{ID: "b", Type: "book", APA7: schema.APA7{Title: "B"}, Annotation: schema.Annotation{Summary: "s2", Keywords: []string{"go", "yaml"}}}

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

	// Filter AND semantics: Go + APA7 should match only e1
	matches := FilterByKeywordsAND(list, []string{"go", "apa7"})
	if len(matches) != 1 || matches[0].ID != "a" {
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
