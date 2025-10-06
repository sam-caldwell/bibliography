package summarize

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type fakeDoer struct {
	status int
	body   string
}

func (f fakeDoer) Do(req *http.Request) (*http.Response, error) {
	// minimal chat completions JSON
	if f.body == "" {
		f.body = `{"choices":[{"message":{"content":"ok"}}]}`
	}
	return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(f.body)), Header: make(http.Header)}, nil
}

func TestSummarizeURL_Success(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "x")
	old := client
	defer func() { client = old }()
	client = fakeDoer{status: 200, body: `{"choices":[{"message":{"content":"A concise summary."}}]}`}
	s, err := SummarizeURL(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("SummarizeURL: %v", err)
	}
	if !strings.Contains(s, "summary") {
		t.Fatalf("unexpected content: %q", s)
	}
}

func TestSummarizeURL_Errors(t *testing.T) {
	// no api key
	os.Unsetenv("OPENAI_API_KEY")
	if _, err := SummarizeURL(context.Background(), "https://x"); err == nil {
		t.Fatalf("expected error when API key missing")
	}
	// http error
	t.Setenv("OPENAI_API_KEY", "x")
	old := client
	defer func() { client = old }()
	client = fakeDoer{status: 500, body: "boom"}
	if _, err := SummarizeURL(context.Background(), "https://x"); err == nil {
		t.Fatalf("expected http error")
	}
}

func TestKeywordsFromTitleAndSummary_ParseJSON(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	old := client
	defer func() { client = old }()
	arr := []string{"Go", "YAML", "Testing"}
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": toJSONString(arr)}}}})
	client = fakeDoer{status: 200, body: string(b)}
	ks, err := KeywordsFromTitleAndSummary(context.Background(), "T", "S")
	if err != nil {
		t.Fatalf("keywords: %v", err)
	}
	if len(ks) != 3 || ks[0] != "go" {
		t.Fatalf("unexpected keywords: %+v", ks)
	}
}

func TestKeywordsFromTitleAndSummary_Fallbacks(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	old := client
	defer func() { client = old }()
	// Non-JSON content string inside JSON with embedded array
	body1, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "Please see: [\\\"foo\\\", \\\"bar\\\"]"}}}})
	client = fakeDoer{status: 200, body: string(body1)}
	ks, err := KeywordsFromTitleAndSummary(context.Background(), "T", "S")
	if err != nil || len(ks) != 2 {
		t.Fatalf("fallback embedded array failed: %+v, %v", ks, err)
	}
	// Garbage -> comma split salvage
	body2, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "foo, bar, baz"}}}})
	client = fakeDoer{status: 200, body: string(body2)}
	ks, err = KeywordsFromTitleAndSummary(context.Background(), "T", "S")
	if err != nil || len(ks) != 3 {
		t.Fatalf("fallback comma split failed: %+v, %v", ks, err)
	}
}

// helper to produce a JSON array string
func toJSONString(arr []string) string {
	b, _ := json.Marshal(arr)
	return string(b)
}
