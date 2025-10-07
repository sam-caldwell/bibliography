package editcmd

import (
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bytes"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditCommand_PrintAndUpdate(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	e := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "Old Title"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	if _, err := store.WriteEntry(e); err != nil {
		t.Fatal(err)
	}
	cmd := New()
	// Print YAML when no assignments
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{"--id", e.ID}); err != nil {
		t.Fatalf("edit print: %v", err)
	}
	if !strings.Contains(out.String(), "Old Title") {
		t.Fatalf("expected YAML output to contain title; got %q", out.String())
	}
	// Update title
	out.Reset()
	if err := cmd.RunE(cmd, []string{"--id", e.ID, "apa7.title=New Title"}); err != nil {
		t.Fatalf("edit update: %v", err)
	}
	// Verify changed
	path := filepath.Join("data", "citations", store.SegmentForType(e.Type), e.ID+".yaml")
	b, _ := os.ReadFile(path)
	var e2 schema.Entry
	_ = yaml.Unmarshal(b, &e2)
	if e2.APA7.Title != "New Title" {
		t.Fatalf("title not updated: %+v", e2)
	}
}

func TestEditCommand_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Create invalid YAML file named by id
	id := schema.NewID()
	citdir := filepath.Join("data", "citations")
	_ = os.MkdirAll(citdir, 0o755)
	if err := os.WriteFile(filepath.Join(citdir, id+".yaml"), []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := New()
	if err := cmd.RunE(cmd, []string{"--id", id, "apa7.title=New"}); err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}

func TestEditCommand_MovePathOnTypeChange(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Write a book entry
	e := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "T"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"book"}}}
	oldPath, err := store.WriteEntry(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("missing old path: %v", err)
	}
	// Change type -> website (moves from books -> site)
	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{"--id", e.ID, "type=website", "apa7.url=https://example.com"}); err != nil {
		t.Fatalf("edit move: %v", err)
	}
	// Old path removed and new path exists
	if _, err := os.Stat(oldPath); err == nil {
		t.Fatalf("old path still exists: %s", oldPath)
	}
	newPath := filepath.Join("data", "citations", "site", e.ID+".yaml")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new path missing: %v", err)
	}
	// Output includes moved and updated lines
	s := out.String()
	if !strings.Contains(s, "moved ") || !strings.Contains(s, "updated ") {
		t.Fatalf("missing moved/updated in output: %q", s)
	}
}
