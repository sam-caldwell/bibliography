package citecmd

import (
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCiteCommand_PrintsCitationAndInline(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	e := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "Hello", URL: "https://example", Accessed: "2024-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	if _, err := store.WriteEntry(e); err != nil {
		t.Fatal(err)
	}

	cmd := New()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.RunE(cmd, []string{e.ID}); err != nil {
		t.Fatalf("cite run: %v", err)
	}
	out := buf.String()
	if out == "" || !bytes.Contains(buf.Bytes(), []byte("citation:")) || !bytes.Contains(buf.Bytes(), []byte("in text:")) {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCiteCommand_ErrorsOnMissingID(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	cmd := New()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	// Unknown id
	if err := cmd.RunE(cmd, []string{"not-found"}); err == nil {
		t.Fatalf("expected error for unknown id")
	}
	// No data directory also fine; still unknown id error
	if _, err := os.Stat(filepath.Join("data", "citations")); err == nil {
		t.Fatalf("unexpected citations dir present")
	}
}
