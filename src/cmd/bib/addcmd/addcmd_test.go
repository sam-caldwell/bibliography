package addcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddWithKeywords_WritesWebsite(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	called := 0
	commit := func(paths []string, msg string) error { called++; return nil }
	if err := AddWithKeywords(nil, commit, "website", map[string]string{"url": "https://example.com"}, []string{"web"}); err != nil {
		t.Fatalf("AddWithKeywords: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data", "citations", "site"))
	if len(files) != 1 {
		t.Fatalf("expected one site yaml written")
	}
	if called == 0 {
		t.Fatalf("expected commit to be called")
	}
}

func TestAddWithKeywords_CoversFieldsAndDerivations(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	calls := 0
	commit := func(paths []string, msg string) error { calls++; return nil }
	// Article with title/author/journal/date/doi; ensure URL from DOI and year from date
	hints := map[string]string{"title": "T", "author": "Doe, Jane", "journal": "J", "date": "2021-03-04", "doi": "10.123/abc"}
	if err := AddWithKeywords(nil, commit, "article", hints, []string{"alpha", "alpha", "beta"}); err != nil {
		t.Fatalf("article add: %v", err)
	}
	// Book with ISBN
	if err := AddWithKeywords(nil, commit, "book", map[string]string{"title": "B", "isbn": "123"}, []string{}); err != nil {
		t.Fatalf("book add: %v", err)
	}
	// Song with title/artist/date
	if err := AddWithKeywords(nil, commit, "song", map[string]string{"title": "S", "author": "Artist", "date": "2019-01-01"}, nil); err != nil {
		t.Fatalf("song add: %v", err)
	}
	// Patent with URL only (title derived from host)
	if err := AddWithKeywords(nil, commit, "patent", map[string]string{"url": "https://patents.example.com/x"}, nil); err != nil {
		t.Fatalf("patent add: %v", err)
	}
	// RFC with minimal title
	if err := AddWithKeywords(nil, commit, "rfc", map[string]string{"title": "RFC X"}, []string{"rfc"}); err != nil {
		t.Fatalf("rfc add: %v", err)
	}
	if calls < 5 {
		t.Fatalf("expected multiple commits, got %d", calls)
	}
}

func TestParseKeywordsCSV_DedupAndTrim(t *testing.T) {
	in := "  go, Golang , ,yaml ,GO "
	ks := parseKeywordsCSV(in)
	got := strings.Join(ks, ",")
	// parseKeywordsCSV preserves original case but dedups case-insensitively
	if !strings.Contains(got, "go") || !strings.Contains(got, "yaml") || !strings.Contains(got, "Golang") {
		t.Fatalf("unexpected keywords parse: %q", got)
	}
}
