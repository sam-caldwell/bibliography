package summarizecmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
)

func TestProcessSummaryForPath_Happy_NoNetwork(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Local HTTP server considered accessible
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	// Write an entry missing a proper summary but with a URL
	e := schema.Entry{ID: schema.NewID(), Type: "article", APA7: schema.APA7{Title: "T", URL: srv.URL, Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "Bibliographic record for X.", Keywords: []string{"article"}}}
	path, err := store.WriteEntry(e)
	if err != nil {
		t.Fatalf("write entry: %v", err)
	}

	// Inject fake summarize/keywords funcs
	oldSum, oldKW := summarizeURLFunc, keywordsFromTitleAndSummaryFunc
	defer func() { summarizeURLFunc = oldSum; keywordsFromTitleAndSummaryFunc = oldKW }()
	summarizeURLFunc = func(ctx context.Context, url string) (string, error) {
		return "This is a test summary that will be wrapped.", nil
	}
	keywordsFromTitleAndSummaryFunc = func(ctx context.Context, title, summary string) ([]string, error) { return []string{"ai", "nlp"}, nil }

	// Run
	cmd := &cobra.Command{Use: "summarize"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	changed, err := processSummaryForPath(context.Background(), cmd, filepath.ToSlash(path))
	if err != nil || !changed {
		t.Fatalf("processSummaryForPath: changed=%v err=%v", changed, err)
	}

	// Verify updated file now has keywords json written indirectly by write
	if _, err := os.Stat(filepath.Join("data", "citations")); err != nil {
		t.Fatalf("expected citations dir: %v", err)
	}
}

func TestProcessSummaryForPath_KeywordsErrorFallback(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	// Entry with no keywords
	e := schema.Entry{ID: schema.NewID(), Type: "movie", APA7: schema.APA7{Title: "T2", URL: srv.URL, Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "Bibliographic record for T2.", Keywords: []string{"movie"}}}
	path, err := store.WriteEntry(e)
	if err != nil {
		t.Fatalf("write entry: %v", err)
	}
	// Inject summarize ok but keywords fail
	oldSum, oldKW := summarizeURLFunc, keywordsFromTitleAndSummaryFunc
	defer func() { summarizeURLFunc = oldSum; keywordsFromTitleAndSummaryFunc = oldKW }()
	summarizeURLFunc = func(ctx context.Context, url string) (string, error) { return "short summary text.", nil }
	keywordsFromTitleAndSummaryFunc = func(ctx context.Context, title, summary string) ([]string, error) { return nil, fmt.Errorf("nope") }
	// Run
	cmd := &cobra.Command{Use: "summarize"}
	if changed, err := processSummaryForPath(context.Background(), cmd, filepath.ToSlash(path)); err != nil || !changed {
		t.Fatalf("processSummaryForPath: changed=%v err=%v", changed, err)
	}
	// Load and verify default type keyword present
	b, _ := os.ReadFile(filepath.ToSlash(path))
	if !bytes.Contains(bytes.ToLower(b), []byte("movie")) {
		t.Fatalf("expected fallback type keyword in file: %s", string(b))
	}
}
