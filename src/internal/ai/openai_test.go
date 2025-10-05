package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestExtractText(t *testing.T) {
	js := `{"output_text": ["line1\n","line2"]}`
	got := extractText(js)
	if got != "line1\nline2" {
		t.Fatalf("extractText got %q", got)
	}
}

func TestGenerateYAML_WithStubbedRun(t *testing.T) {
	// Stub runCommand to return JSON carrying YAML
	old := runCommand
	t.Cleanup(func() { runCommand = old })
	yaml := "id: abc\ntype: website\napa7:\n  title: 'Hello'\nannotation:\n  summary: 's'\n  keywords: ['k']\n"
	runCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		// Return JSON with output_text as a string
		return "{" + "\"output_text\": " + fmt.Sprintf("%q", yaml) + "}", nil
	}
	g := &openAIGenerator{model: "gpt-4.1-mini", apiKey: "x"}
	e, raw, err := g.GenerateYAML(context.Background(), "website", map[string]string{"url": "https://x"})
	if err != nil {
		t.Fatalf("GenerateYAML: %v", err)
	}
	if raw == "" || e.ID != "abc" || e.APA7.Title != "Hello" {
		t.Fatalf("bad parse: %+v raw=%q", e, raw)
	}
}

func TestGenerateYAML_SlugOnMissingID(t *testing.T) {
	old := runCommand
	t.Cleanup(func() { runCommand = old })
	yaml := "type: website\napa7:\n  title: 'Hello World'\nannotation:\n  summary: 's'\n  keywords: ['k']\n"
	runCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "{" + "\"output_text\": " + fmt.Sprintf("%q", yaml) + "}", nil
	}
	g := &openAIGenerator{model: "gpt-4.1-mini", apiKey: "x"}
	e, _, err := g.GenerateYAML(context.Background(), "website", nil)
	if err != nil {
		t.Fatalf("GenerateYAML: %v", err)
	}
	if e.ID != "hello-world" {
		t.Fatalf("expected slug id, got %q", e.ID)
	}
}

func TestNewGeneratorFromEnv_MissingKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := NewGeneratorFromEnv("gpt-4.1-mini"); err == nil {
		t.Fatalf("expected error for missing api key")
	}
}

func TestGenerateYAML_InvalidYAML(t *testing.T) {
	old := runCommand
	t.Cleanup(func() { runCommand = old })
	runCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "{\"output_text\": \"not: [valid\"}", nil
	}
	g := &openAIGenerator{model: "gpt-4.1-mini", apiKey: "x"}
	if _, _, err := g.GenerateYAML(context.Background(), "website", nil); err == nil {
		t.Fatalf("expected YAML parse error")
	}
}

func TestGenerateYAML_EmptyOutput(t *testing.T) {
	old := runCommand
	t.Cleanup(func() { runCommand = old })
	runCommand = func(_ context.Context, _ string, _ []string) (string, error) {
		return "{}", nil
	}
	g := &openAIGenerator{model: "gpt-4.1-mini", apiKey: "x"}
	if _, _, err := g.GenerateYAML(context.Background(), "website", nil); err == nil {
		t.Fatalf("expected error for empty YAML output")
	}
}

func TestBuildUserPromptContainsHints(t *testing.T) {
	s := buildUserPrompt("book", map[string]string{"title": "X", "isbn": "123"})
	if !strings.Contains(s, "type=book") || !strings.Contains(s, "title: X") || !strings.Contains(s, "isbn: 123") {
		t.Fatalf("prompt missing expected content: %q", s)
	}
}

func TestExtractTextVariants(t *testing.T) {
	// output array variant
	js := `{"output":[{"content":[{"text":"a"},{"text":"b"}]}]}`
	if got := extractText(js); got != "ab" {
		t.Fatalf("got %q", got)
	}
	// content array at top-level
	js2 := `{"content":[{"text":"x"},{"text":"y"}]}`
	if got := extractText(js2); got != "xy" {
		t.Fatalf("got %q", got)
	}
	// invalid json -> fallback
	if got := extractText("not-json"); got != "not-json" {
		t.Fatalf("fallback mismatch")
	}
}

func TestNewGeneratorFromEnv_Success(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "x")
	if _, err := NewGeneratorFromEnv("gpt-4.1-mini"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
