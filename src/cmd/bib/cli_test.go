package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"bibliography/src/internal/doi"
	"bibliography/src/internal/openlibrary"
	"bibliography/src/internal/schema"
)

// test HTTP client for OpenLibrary injection
type testHTTPDoer struct {
	status int
	body   string
}

func (t testHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	r := &http.Response{StatusCode: t.status, Body: io.NopCloser(strings.NewReader(t.body)), Header: make(http.Header)}
	return r, nil
}

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

func TestLookupSite_Basic(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Fake git commit/push (do nothing)
	called := false
	commitAndPush = func(paths []string, msg string) error { called = true; return nil }
	t.Cleanup(func() { commitAndPush = nil })

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())

	// Run lookup site
	if _, err := execCmd(rootCmd, "lookup", "site", "https://example.com"); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "example-com.yaml")); err != nil {
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

func TestLookupBookAndMovie_Minimal(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	commitAndPush = func(paths []string, msg string) error { return nil }

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())

	if _, err := execCmd(rootCmd, "lookup", "book", "--name", "The Book"); err != nil {
		t.Fatalf("lookup book: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "the-book.yaml")); err != nil {
		t.Fatalf("book yaml missing: %v", err)
	}

	if _, err := execCmd(rootCmd, "lookup", "movie", "Best", "Movie", "--date", "2024-01-01"); err != nil {
		t.Fatalf("lookup movie: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "best-movie-2024.yaml")); err != nil {
		t.Fatalf("movie yaml missing: %v", err)
	}
}

func intPtr(v int) *int { return &v }

// Removed: requirement for OPENAI_API_KEY

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

	// No OpenAI path; the command constructs minimal entry from URL

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
	b, err := os.ReadFile(filepath.Join("data/citations", "t.yaml"))
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	if !bytes.Contains(b, []byte("accessed:")) {
		t.Fatalf("expected accessed set in yaml: %s", string(b))
	}
}

func TestLookupArticleByDOI_NoOpenAI(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	commitAndPush = func(paths []string, msg string) error { return nil }
	// Stub doi.org CSL JSON response
	csl := `{
		"title": "My Article",
		"author": [{"family":"Doe","given":"Jane Q"}],
		"container-title": "Journal of Tests",
		"issued": {"date-parts": [[2023,5,2]]},
		"DOI": "10.1234/x",
		"volume": "12",
		"issue": "3",
		"page": "45-60",
		"publisher": "TestPub"
	}`
	doi.SetHTTPClient(testHTTPDoer{status: 200, body: csl})
	t.Cleanup(func() { doi.SetHTTPClient(&http.Client{}) })

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "article", "--doi", "10.1234/x"); err != nil {
		t.Fatalf("lookup article: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "my-article-2023.yaml")); err != nil {
		t.Fatalf("article yaml missing: %v", err)
	}
}

func TestLookupArticleByMetadata_Minimal(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	commitAndPush = func(paths []string, msg string) error { return nil }

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "article", "--title", "X", "--author", "Doe, J.", "--journal", "J", "--date", "2023-01-01"); err != nil {
		t.Fatalf("lookup article by metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "x-2023.yaml")); err != nil {
		t.Fatalf("article yaml missing: %v", err)
	}
}

func TestLookupBookByISBN_OpenLibrary(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	commitAndPush = func(paths []string, msg string) error { return nil }

	// Stub OpenLibrary HTTP client
	body := `{"ISBN:123": {"title":"Name","publish_date":"2001","publishers":[{"name":"Pub"}],"authors":[{"name":"John Smith"}],"subjects":[{"name":"Topic"}],"url":"https://openlibrary.org/books/OL1M/Name"}}`
	// swap client
	openlibrary.SetHTTPClient(testHTTPDoer{status: 200, body: body})
	t.Cleanup(func() { openlibrary.SetHTTPClient(&http.Client{}) })

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "book", "--name", "Name", "--author", "Smith, J.", "--isbn", "123", "--keywords", "k1,k2"); err != nil {
		t.Fatalf("lookup book by isbn: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "name-2001.yaml")); err != nil {
		t.Fatalf("book yaml missing: %v", err)
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("FOO_BAR", "baz")
	if v := getEnv("FOO_BAR", "zzz"); v != "baz" {
		t.Fatalf("expected env value, got %q", v)
	}
	t.Setenv("FOO_BAR", "")
	if v := getEnv("FOO_BAR", "zzz"); v != "zzz" {
		t.Fatalf("expected default value, got %q", v)
	}
}

func TestIndexErrorsOnInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	if err := os.MkdirAll("data/citations", 0o755); err != nil {
		t.Fatal(err)
	}
	// Write invalid YAML
	if err := os.WriteFile(filepath.Join("data/citations", "bad.yaml"), []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newIndexCmd())
	if _, err := execCmd(rootCmd, "index"); err == nil {
		t.Fatalf("expected error for invalid YAML during index")
	}
}

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, fmt.Errorf("write err") }

func TestIndexOutputWriteError(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// No citations; index will still attempt to write keywords.json
	cmd := newIndexCmd()
	cmd.SetOut(errWriter{})
	if err := cmd.RunE(cmd, []string{}); err == nil {
		t.Fatalf("expected error when writing output")
	}
}

func TestLookupSite_MissingArg(t *testing.T) {
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "site"); err == nil {
		t.Fatalf("expected error for missing site arg")
	}
}

func TestLookupBook_ComputesSlug(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	commitAndPush = func(paths []string, msg string) error { return nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "lookup", "book", "--name", "Hello World"); err != nil {
		t.Fatalf("lookup book: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "hello-world.yaml")); err != nil {
		t.Fatalf("expected hello-world.yaml written: %v", err)
	}
}

func TestExecuteFunction(t *testing.T) {
	// Replace rootCmd with a dummy that has a no-op Run to avoid os.Exit
	ran := false
	rootCmd = &cobra.Command{Use: "bib", Run: func(cmd *cobra.Command, args []string) { ran = true }}
	if err := execute(); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !ran {
		t.Fatalf("expected root command to run in execute")
	}
}

func TestExecuteReturnsError(t *testing.T) {
	rootCmd = &cobra.Command{Use: "bib", RunE: func(cmd *cobra.Command, args []string) error { return fmt.Errorf("boom") }}
	if err := execute(); err == nil {
		t.Fatalf("expected execute to return error")
	}
}

// Removed OpenAI generator error tests

func TestDoLookup_WriteEntryError(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Create a file at data/citations to break directory creation
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/citations", []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := doLookup(context.Background(), "website", map[string]string{"url": "https://x", "title": "T"}); err == nil {
		t.Fatalf("expected error when write entry fails")
	}
}

// Compile-time ensure doLookup uses context, no-op just to silence unused in coverage.
var _ = context.Background()
