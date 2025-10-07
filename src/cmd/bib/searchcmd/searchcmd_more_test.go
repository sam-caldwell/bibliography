package searchcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
)

func TestSearch_DateComparisons_AndAllContains(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Seed entries with different dates/years and publishers
	y2018 := 2018
	e1 := schema.Entry{ID: schema.NewID(), Type: "article", APA7: schema.APA7{Title: "A1", Date: "2020-05-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	e2 := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "A2", Year: &y2018, Publisher: "Acme Co"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	if _, err := store.WriteEntry(e1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteEntry(e2); err != nil {
		t.Fatal(err)
	}

	// year>=2020 should match e1 only
	cmd := New()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.RunE(cmd, []string{"year>=2020"}); err != nil {
		t.Fatalf("expr run: %v", err)
	}
	out := buf.String()
	if out == "" || !bytes.Contains(buf.Bytes(), []byte(e1.ID)) || bytes.Contains(buf.Bytes(), []byte(e2.ID)) {
		t.Fatalf("date compare mismatch output: %q", out)
	}

	// all~=acme should match e2 only via YAML contains
	cmd2 := New()
	buf.Reset()
	cmd2.SetOut(&buf)
	if err := cmd2.RunE(cmd2, []string{"all~=acme"}); err != nil {
		t.Fatalf("all contains: %v", err)
	}
	out2 := buf.String()
	if out2 == "" || !bytes.Contains(buf.Bytes(), []byte(e2.ID)) {
		t.Fatalf("expected e2 in all search: %q", out2)
	}

	// Also verify flag path --all matches
	cmd3 := New()
	buf.Reset()
	cmd3.SetOut(&buf)
	cmd3.SetArgs([]string{"--all", "Acme"})
	if err := cmd3.Execute(); err != nil {
		t.Fatalf("flag all execute: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected output for --all")
	}

	_ = filepath.ToSlash
}
