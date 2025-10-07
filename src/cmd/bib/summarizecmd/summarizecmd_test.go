package summarizecmd

import (
    "context"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/spf13/cobra"

    "bibliography/src/internal/schema"
    "bibliography/src/internal/store"
)

func TestWrapText(t *testing.T) {
	s := "alpha beta gamma delta"
	out := wrapText(s, 10)
	if out == "" || !strings.Contains(out, "\n") {
		t.Fatalf("wrapText did not wrap: %q", out)
	}
}

func TestMergeSortDedupKeywords(t *testing.T) {
	out := mergeSortDedupKeywords([]string{"Go", "yaml"}, []string{"go", "cli"}, "tool")
	// all lowercase and deduped
	got := strings.Join(out, ",")
	for _, w := range []string{"go", "yaml", "cli", "tool"} {
		if !strings.Contains(got, w) {
			t.Fatalf("missing keyword %q in %q", w, got)
		}
	}
}

func TestURLAccessible(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
    defer srv.Close()
    if !urlAccessible(context.Background(), srv.URL) {
        t.Fatalf("expected urlAccessible true for test server")
    }
}

func TestURLAccessible_HeadFailsGetSucceeds(t *testing.T) {
    // HEAD returns 405, GET returns 200
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodHead {
            w.WriteHeader(405)
            return
        }
        w.WriteHeader(200)
    }))
    defer srv.Close()
    if !urlAccessible(context.Background(), srv.URL) {
        t.Fatalf("expected true when GET succeeds after HEAD fails")
    }
}

func TestGatherCitationPaths_FiltersYAML(t *testing.T) {
    dir := t.TempDir(); old,_ := os.Getwd(); t.Cleanup(func(){ _ = os.Chdir(old) }); _ = os.Chdir(dir)
    // Seed mixed files
    _ = os.MkdirAll(filepath.Join("data","citations","site"), 0o755)
    _ = os.WriteFile(filepath.Join("data","citations","site","a.yaml"), []byte("{}"), 0o644)
    _ = os.WriteFile(filepath.Join("data","citations","site","b.txt"), []byte("x"), 0o644)
    // Another directory
    _ = os.MkdirAll(filepath.Join("data","citations","book"), 0o755)
    _ = os.WriteFile(filepath.Join("data","citations","book","c.yaml"), []byte("{}"), 0o644)
    paths, err := gatherCitationPaths()
    if err != nil { t.Fatal(err) }
    // Should include only the two YAML files
    if len(paths) != 2 { t.Fatalf("want 2 yaml paths, got %d: %+v", len(paths), paths) }
}

func TestProcessSummary_SkipOnUnreachableAndSummarizeError(t *testing.T) {
    dir := t.TempDir(); old,_ := os.Getwd(); t.Cleanup(func(){ _ = os.Chdir(old) }); _ = os.Chdir(dir)
    // Unreachable URL server (always 500)
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
    defer srv.Close()
    e := schema.Entry{ID: schema.NewID(), Type: "article", APA7: schema.APA7{Title: "T", URL: srv.URL, Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "Bibliographic record for T.", Keywords: []string{"article"}}}
    p, _ := store.WriteEntry(e)
    cmd := &cobra.Command{Use: "summarize"}
    var errBuf strings.Builder
    cmd.SetErr(&errBuf)
    changed, err := processSummaryForPath(context.Background(), cmd, filepath.ToSlash(p))
    if err != nil || changed { t.Fatalf("expected skip: changed=%v err=%v", changed, err) }
    if !strings.Contains(errBuf.String(), "skip ") { t.Fatalf("expected skip message, got: %q", errBuf.String()) }

    // Accessible URL but summarize returns error -> also skip
    srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
    defer srv2.Close()
    e2 := schema.Entry{ID: schema.NewID(), Type: "article", APA7: schema.APA7{Title: "T2", URL: srv2.URL, Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "Bibliographic record for T2.", Keywords: []string{"article"}}}
    p2, _ := store.WriteEntry(e2)
    oldS := summarizeURLFunc
    summarizeURLFunc = func(ctx context.Context, url string) (string, error) { return "", fmt.Errorf("nope") }
    defer func(){ summarizeURLFunc = oldS }()
    errBuf.Reset()
    changed, err = processSummaryForPath(context.Background(), cmd, filepath.ToSlash(p2))
    if err != nil || changed { t.Fatalf("expected skip on summarize error: changed=%v err=%v", changed, err) }
}

func TestNeedsSummaryVariants(t *testing.T) {
	if !needsSummary(schema.Entry{Annotation: schema.Annotation{Summary: ""}}) {
		t.Fatalf("empty should need summary")
	}
	if !needsSummary(schema.Entry{Annotation: schema.Annotation{Summary: "Bibliographic record for X."}}) {
		t.Fatalf("placeholder should need summary")
	}
	if needsSummary(schema.Entry{Annotation: schema.Annotation{Summary: "Actual summary."}}) {
		t.Fatalf("real summary should not need summary")
	}
}

func TestLoadEntryIfSummarizable_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// invalid YAML but should return ok=false and no error (parse error suppressed)
	p := filepath.Join("data", "citations", "x.yaml")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte("not: [valid"), 0o644)
	if _, ok, err := loadEntryIfSummarizable(p); err != nil || ok {
		t.Fatalf("expected ok=false err=nil; got ok=%v err=%v", ok, err)
	}
}
