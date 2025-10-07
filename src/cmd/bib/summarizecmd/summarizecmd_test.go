package summarizecmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
