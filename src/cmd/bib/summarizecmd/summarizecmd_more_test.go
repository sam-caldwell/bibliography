package summarizecmd

import (
	"bibliography/src/internal/schema"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestGatherCitationPathsAndLoadSummarizable(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	_ = os.MkdirAll(filepath.Join("data", "citations"), 0o755)
	// write a minimal summarizable entry (has URL + placeholder summary)
	e := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "T", URL: "https://x", Accessed: "2024-01-01"}, Annotation: schema.Annotation{Summary: "Bibliographic record for X.", Keywords: []string{"k"}}}
	b, _ := yaml.Marshal(e)
	_ = os.WriteFile(filepath.Join("data", "citations", e.ID+".yaml"), b, 0o644)
	// write a non-summarizable entry (no URL)
	e2 := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "B"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	b2, _ := yaml.Marshal(e2)
	_ = os.WriteFile(filepath.Join("data", "citations", e2.ID+".yaml"), b2, 0o644)

	paths, err := gatherCitationPaths()
	if err != nil || len(paths) < 1 {
		t.Fatalf("gather paths: %v, %v", paths, err)
	}
	e3, ok, err := loadEntryIfSummarizable(paths[0])
	if err != nil {
		t.Fatalf("load summarizable: %v", err)
	}
	_ = e3
	_ = ok // just ensure functions run
}
