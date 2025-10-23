package addcmd

import (
	"bibliography/src/internal/store"
	"bytes"
	"os"
	"testing"
)

func TestBuilder_RunSomeSubcommands(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	commits := 0
	b := New(func(paths []string, msg string) error { commits++; return nil })

	// site
	site := b.Site()
	site.SetArgs([]string{"https://example.com"})
	site.SetOut(new(bytes.Buffer))
	if err := site.Execute(); err != nil {
		t.Fatalf("site: %v", err)
	}
	if _, err := os.Stat(store.BibFile); err != nil {
		t.Fatalf("bib missing: %v", err)
	}

	// book via hints flags (no external lookup)
	book := b.Book()
	book.SetArgs([]string{"--name", "B", "--author", "Doe, J."})
	book.SetOut(new(bytes.Buffer))
	if err := book.Execute(); err != nil {
		t.Fatalf("book: %v", err)
	}

	// article via hints flags
	art := b.Article()
	art.SetArgs([]string{"--title", "T", "--author", "Doe, J.", "--journal", "J", "--date", "2022-01-01"})
	art.SetOut(new(bytes.Buffer))
	if err := art.Execute(); err != nil {
		t.Fatalf("article: %v", err)
	}

	// movie via positional arg (falls back to hints path when providers fail)
	mov := b.Movie()
	mov.SetArgs([]string{"A Movie"})
	mov.SetOut(new(bytes.Buffer))
	if err := mov.Execute(); err != nil {
		t.Fatalf("movie: %v", err)
	}

	// song via positional arg
	song := b.Song()
	song.SetArgs([]string{"A Song", "--artist", "Artist"})
	song.SetOut(new(bytes.Buffer))
	if err := song.Execute(); err != nil {
		t.Fatalf("song: %v", err)
	}

	// patent via URL flag
	pat := b.Patent()
	pat.SetArgs([]string{"--url", "https://patents.example.com/p"})
	pat.SetOut(new(bytes.Buffer))
	if err := pat.Execute(); err != nil {
		t.Fatalf("patent: %v", err)
	}

	if commits < 6 {
		t.Fatalf("expected multiple commits, got %d", commits)
	}
}
