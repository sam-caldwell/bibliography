package exportcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type entry struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	APA7       apa7   `json:"apa7"`
	Annotation annot  `json:"annotation"`
}
type apa7 struct {
	Title    string   `json:"title"`
	URL      string   `json:"url,omitempty"`
	Accessed string   `json:"accessed,omitempty"`
	Authors  []author `json:"authors"`
}
type author struct{ Family, Given string }
type annot struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
}

func TestExportBibWritesFile(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Create a minimal YAML (JSON) entry file under data/citations
	path := filepath.Join("data", "citations", "site", "x.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	e := entry{ID: "00000000-0000-4000-8000-000000000001", Type: "website", APA7: apa7{Title: "Hello", URL: "https://e", Accessed: "2025-01-01", Authors: []author{{Family: "Corp"}}}, Annotation: annot{Summary: "s", Keywords: []string{"k"}}}
	b, _ := json.Marshal(e)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cmd := New()
	// Use default output (data/library.bib)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data", "library.bib")); err != nil {
		t.Fatalf("missing library.bib: %v", err)
	}
}
