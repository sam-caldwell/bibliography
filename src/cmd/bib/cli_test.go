package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"bibliography/src/internal/ai"
	"bibliography/src/internal/schema"
)

// Helper to execute a Cobra command and capture stdout/stderr
func execCmd(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestIndexAndSearch(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Seed two entries
	if err := os.MkdirAll("data/citations", 0o755); err != nil {
		t.Fatal(err)
	}
	e1 := schema.Entry{ID: "a", Type: "website", APA7: schema.APA7{Title: "A", URL: "https://a", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"golang", "apa7"}}}
	e2 := schema.Entry{ID: "b", Type: "book", APA7: schema.APA7{Title: "B"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"golang", "yaml"}}}
	write := func(e schema.Entry) {
		path := filepath.Join("data/citations", e.ID+".yaml")
		f, _ := os.Create(path)
		defer f.Close()
		// minimal yaml
		_, _ = f.WriteString("id: " + e.ID + "\n")
		_, _ = f.WriteString("type: " + e.Type + "\n")
		_, _ = f.WriteString("apa7:\n  title: \"" + e.APA7.Title + "\"\n")
		if e.APA7.URL != "" {
			_, _ = f.WriteString("  url: \"" + e.APA7.URL + "\"\n")
			_, _ = f.WriteString("  accessed: \"" + e.APA7.Accessed + "\"\n")
		}
		_, _ = f.WriteString("annotation:\n  summary: \"s\"\n  keywords: [\"" + e.Annotation.Keywords[0] + "\", \"" + e.Annotation.Keywords[1] + "\"]\n")
	}
	write(e1)
	write(e2)

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newIndexCmd(), newSearchCmd())

	// Run index
	if _, err := execCmd(rootCmd, "index"); err != nil {
		t.Fatalf("index: %v", err)
	}
	if _, err := os.Stat("data/metadata/keywords.json"); err != nil {
		t.Fatalf("keywords.json: %v", err)
	}

	// Search AND
	out, err := execCmd(rootCmd, "search", "--keyword", "golang,apa7")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("a:")) {
		t.Fatalf("expected match a in output: %q", out)
	}
}

func TestLookupWithFakeAI(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Fake AI
	e := schema.Entry{ID: "example-2025", Type: "website", APA7: schema.APA7{Title: "Example", URL: "https://example.com", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"golang"}}}
	fake := &ai.FakeGenerator{Entry: e, Raw: "raw"}
	newGenerator = func(model string) (ai.Generator, error) { return fake, nil }
	t.Cleanup(func() {
		newGenerator = func(model string) (ai.Generator, error) { return ai.NewGeneratorFromEnv(model) }
	})

	// Fake git commit/push (do nothing)
	called := false
	commitAndPush = func(paths []string, msg string) error { called = true; return nil }
	t.Cleanup(func() { commitAndPush = nil })

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())

	// Set dummy key to pass check
	os.Setenv("OPENAI_API_KEY", "dummy")
	t.Cleanup(func() { os.Unsetenv("OPENAI_API_KEY") })

	// Run lookup site
	if _, err := execCmd(rootCmd, "lookup", "site", "https://example.com"); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", e.ID+".yaml")); err != nil {
		t.Fatalf("expected citation yaml written: %v", err)
	}
	if !called {
		t.Fatalf("expected commitAndPush to be called")
	}
}

func TestSearchFlagValidation(t *testing.T) {
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newSearchCmd())
	if _, err := execCmd(rootCmd, "search"); err == nil {
		t.Fatalf("expected error when --keyword missing")
	}
}

func TestLookupBookAndMovie_FakeAI(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	os.Setenv("OPENAI_API_KEY", "dummy")
	t.Cleanup(func() { os.Unsetenv("OPENAI_API_KEY") })

	// Fake AI returns book/movie with missing accessed when no URL
	fake := &ai.FakeGenerator{Entry: schema.Entry{ID: "the-book", Type: "book", APA7: schema.APA7{Title: "The Book"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}}
	newGenerator = func(model string) (ai.Generator, error) { return fake, nil }
	t.Cleanup(func() {
		newGenerator = func(model string) (ai.Generator, error) { return ai.NewGeneratorFromEnv(model) }
	})
	commitAndPush = func(paths []string, msg string) error { return nil }

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())

	if _, err := execCmd(rootCmd, "lookup", "book", "--name", "The Book"); err != nil {
		t.Fatalf("lookup book: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "the-book.yaml")); err != nil {
		t.Fatalf("book yaml missing: %v", err)
	}

	// Movie
	fake.Entry = schema.Entry{ID: "best-movie-2024", Type: "movie", APA7: schema.APA7{Title: "Best Movie", Year: intPtr(2024)}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	if _, err := execCmd(rootCmd, "lookup", "movie", "Best", "Movie", "--date", "2024-01-01"); err != nil {
		t.Fatalf("lookup movie: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "best-movie-2024.yaml")); err != nil {
		t.Fatalf("movie yaml missing: %v", err)
	}
}

func intPtr(v int) *int { return &v }

func TestLookupRequiresAPIKey(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	t.Setenv("OPENAI_API_KEY", "")
	// Provide newGenerator anyway but it shouldn't be used due to missing key
	newGenerator = func(model string) (ai.Generator, error) { return &ai.FakeGenerator{}, nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "site", "https://example.com"); err == nil {
		t.Fatalf("expected error when OPENAI_API_KEY missing")
	}
}

func TestIndexPrintsPath(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newIndexCmd())
	out, err := execCmd(rootCmd, "index")
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("wrote data/metadata/keywords.json")) {
		t.Fatalf("expected output to mention keywords.json, got %q", out)
	}
}

func TestLookupSite_SetsAccessedAndHandlesCommitError(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	os.Setenv("OPENAI_API_KEY", "dummy")
	t.Cleanup(func() { os.Unsetenv("OPENAI_API_KEY") })

	// Fake AI returns URL without accessed
	e := schema.Entry{ID: "site", Type: "website", APA7: schema.APA7{Title: "T", URL: "https://t"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
	newGenerator = func(model string) (ai.Generator, error) { return &ai.FakeGenerator{Entry: e}, nil }
	t.Cleanup(func() {
		newGenerator = func(model string) (ai.Generator, error) { return ai.NewGeneratorFromEnv(model) }
	})

	// First, simulate commit error
	commitAndPush = func(paths []string, msg string) error { return fmt.Errorf("push failed") }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "site", "https://t"); err == nil {
		t.Fatalf("expected commit error to surface")
	}

	// Now simulate success
	commitAndPush = func(paths []string, msg string) error { return nil }
	if _, err := execCmd(rootCmd, "lookup", "site", "https://t"); err != nil {
		t.Fatalf("lookup site: %v", err)
	}
	// Verify YAML written and accessed set
	b, err := os.ReadFile(filepath.Join("data/citations", "site.yaml"))
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	if !bytes.Contains(b, []byte("accessed:")) {
		t.Fatalf("expected accessed set in yaml: %s", string(b))
	}
}

func TestLookupArticle_FakeAI(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	os.Setenv("OPENAI_API_KEY", "dummy")
	t.Cleanup(func() { os.Unsetenv("OPENAI_API_KEY") })

	fake := &ai.FakeGenerator{Entry: schema.Entry{ID: "my-article-2023", Type: "article", APA7: schema.APA7{Title: "My Article", DOI: "10.1234/x", Year: intPtr(2023)}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}}
	newGenerator = func(model string) (ai.Generator, error) { return fake, nil }
	t.Cleanup(func() {
		newGenerator = func(model string) (ai.Generator, error) { return ai.NewGeneratorFromEnv(model) }
	})
	commitAndPush = func(paths []string, msg string) error { return nil }

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "article", "--doi", "10.1234/x"); err != nil {
		t.Fatalf("lookup article: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "my-article-2023.yaml")); err != nil {
		t.Fatalf("article yaml missing: %v", err)
	}
}

// Compile-time ensure doLookup uses context, no-op just to silence unused in coverage.
var _ = context.Background()
