package verifycmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"

	"bibliography/src/internal/schema"
)

func TestVerifyWithProviders_ArticleURLOnlyEligible(t *testing.T) {
	// Start a local HTTP server that returns minimal HTML with a title
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><head><title>Test Article</title><meta name=\"author\" content=\"Doe, J\"></head><body>ok</body></html>"))
	}))
	defer srv.Close()

	e := schema.Entry{ID: schema.NewID(), Type: "article", APA7: schema.APA7{Title: "T", URL: srv.URL, Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	cmd := &cobra.Command{}
	provs, ok := verifyWithProviders(cmd, e)
	if !ok {
		t.Fatalf("expected url-only article to be eligible; providers=%v", provs)
	}
	// Should include head/get and either web or html providers
	has := func(s string) bool {
		for _, p := range provs {
			if p == s {
				return true
			}
		}
		return false
	}
	if !has("head/get") || !(has("web") || has("html")) {
		t.Fatalf("expected head/get and web|html providers, got %v", provs)
	}
}
