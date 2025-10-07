package summarizecmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bibliography/src/internal/schema"
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
