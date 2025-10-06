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
	rfcpkg "bibliography/src/internal/rfc"
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
	if !bytes.Contains([]byte(out), []byte("data/citations/site/a.yaml:")) {
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

	// Run add site
	if _, err := execCmd(rootCmd, "add", "site", "https://example.com"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "site", "example-com.yaml")); err != nil {
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

	if _, err := execCmd(rootCmd, "add", "book", "--name", "The Book"); err != nil {
		t.Fatalf("add book: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "books", "the-book.yaml")); err != nil {
		t.Fatalf("book yaml missing: %v", err)
	}

	if _, err := execCmd(rootCmd, "add", "movie", "Best", "Movie", "--date", "2024-01-01"); err != nil {
		t.Fatalf("add movie: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "movie", "best-movie-2024.yaml")); err != nil {
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
	if !bytes.Contains([]byte(out), []byte("wrote data/metadata/doi.json")) {
		t.Fatalf("expected output to mention doi.json, got %q", out)
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
	if _, err := execCmd(rootCmd, "add", "site", "https://t"); err == nil {
		t.Fatalf("expected commit error to surface")
	}

	// Now simulate success
	commitAndPush = func(paths []string, msg string) error { return nil }
	if _, err := execCmd(rootCmd, "add", "site", "https://t"); err != nil {
		t.Fatalf("add site: %v", err)
	}
	// Verify YAML written and accessed set
	b, err := os.ReadFile(filepath.Join("data/citations", "site", "t.yaml"))
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
	if _, err := execCmd(rootCmd, "add", "article", "--doi", "10.1234/x"); err != nil {
		t.Fatalf("add article: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "article", "my-article-2023.yaml")); err != nil {
		t.Fatalf("article yaml missing: %v", err)
	}
	// verify DOI and doi.org URL recorded in YAML
	b, err := os.ReadFile(filepath.Join("data/citations", "article", "my-article-2023.yaml"))
	if err != nil {
		t.Fatalf("read article yaml: %v", err)
	}
	if !bytes.Contains(b, []byte("10.1234/x")) {
		t.Fatalf("expected DOI recorded in yaml, got:\n%s", string(b))
	}
	if !bytes.Contains(b, []byte("url: https://doi.org/10.1234/x")) {
		t.Fatalf("expected doi.org URL recorded in yaml, got:\n%s", string(b))
	}
}

func TestLookupArticleByDOI_RecordsProvidedDOIIfMissingFromCSL(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	commitAndPush = func(paths []string, msg string) error { return nil }
	// Stub CSL JSON WITHOUT a DOI field
	csl := `{"title":"T","author":[{"family":"Doe"}],"container-title":"J","issued":{"date-parts":[[2022,1,1]]}}`
	doi.SetHTTPClient(testHTTPDoer{status: 200, body: csl})
	t.Cleanup(func() { doi.SetHTTPClient(&http.Client{}) })

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "add", "article", "--doi", "10.5555/abc"); err != nil {
		t.Fatalf("add article: %v", err)
	}
	// slug should be t-2022.yaml
	p := filepath.Join("data/citations", "article", "t-2022.yaml")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("article yaml missing: %v", err)
	}
	by, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Contains(by, []byte("10.5555/abc")) {
		t.Fatalf("expected fallback DOI recorded, got:\n%s", string(by))
	}
	if !bytes.Contains(by, []byte("https://doi.org/10.5555/abc")) {
		t.Fatalf("expected doi.org URL recorded, got:\n%s", string(by))
	}
}

func TestLookupRFC_Basic(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// stub rfc HTTP (BibTeX)
	bib := `@misc{rfc5424,
  series = {Request for Comments},
  number = 5424,
  howpublished = {RFC 5424},
  publisher = {RFC Editor},
  doi = {10.17487/RFC5424},
  url = {https://www.rfc-editor.org/info/rfc5424},
  author = {Rainer Gerhards},
  title = {{The Syslog Protocol}},
  year = 2009,
  month = mar,
  abstract = {This document describes the syslog protocol.}
}`
	rfcpkg.SetHTTPClient(testHTTPDoer{status: 200, body: bib})
	t.Cleanup(func() { rfcpkg.SetHTTPClient(&http.Client{}) })

	commitAndPush = func(paths []string, msg string) error { return nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newLookupCmd())
	if _, err := execCmd(rootCmd, "add", "rfc", "rfc5424"); err != nil {
		t.Fatalf("add rfc: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "rfc", "rfc5424.yaml")); err != nil {
		t.Fatalf("rfc yaml missing: %v", err)
	}
	by, err := os.ReadFile(filepath.Join("data/citations", "rfc", "rfc5424.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(by, []byte("syslog protocol")) {
		t.Fatalf("expected abstract in YAML summary, got:\n%s", string(by))
	}
	if !bytes.Contains(by, []byte("bibtex_url: https://datatracker.ietf.org/doc/rfc5424/bibtex/")) {
		t.Fatalf("expected bibtex_url in YAML, got:\n%s", string(by))
	}
}

func TestRepairDOICommand_ExtractsFromPublisherURLAndNormalizesURL(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Seed an article with a non-doi.org publisher URL containing the DOI path
	if err := os.MkdirAll("data/citations/article", 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "" +
		"id: a1\n" +
		"type: article\n" +
		"apa7:\n" +
		"  title: T\n" +
		"  url: https://dl.acm.org/doi/10.1145/12345.67890\n" +
		"  accessed: \"2025-01-01\"\n" +
		"annotation:\n" +
		"  summary: s\n" +
		"  keywords: [k]\n"
	if err := os.WriteFile(filepath.Join("data/citations/article", "a1.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run repair-doi
	commitAndPush = func(paths []string, msg string) error { return nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newRepairDOICmd())
	if _, err := execCmd(rootCmd, "repair-doi"); err != nil {
		t.Fatalf("repair-doi: %v", err)
	}
	b, err := os.ReadFile(filepath.Join("data/citations/article", "a1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("doi: 10.1145/12345.67890")) {
		t.Fatalf("expected DOI to be written, got:\n%s", string(b))
	}
	if !bytes.Contains(b, []byte("url: https://doi.org/10.1145/12345.67890")) {
		t.Fatalf("expected doi.org URL, got:\n%s", string(b))
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
	if _, err := execCmd(rootCmd, "add", "article", "--title", "X", "--author", "Doe, J.", "--journal", "J", "--date", "2023-01-01"); err != nil {
		t.Fatalf("add article by metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "article", "x-2023.yaml")); err != nil {
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
	if _, err := execCmd(rootCmd, "add", "book", "--name", "Name", "--author", "Smith, J.", "--isbn", "123", "--keywords", "k1,k2"); err != nil {
		t.Fatalf("add book by isbn: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "books", "name-2001.yaml")); err != nil {
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
	if _, err := execCmd(rootCmd, "add", "site"); err == nil {
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
	if _, err := execCmd(rootCmd, "add", "book", "--name", "Hello World"); err != nil {
		t.Fatalf("add book: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data/citations", "books", "hello-world.yaml")); err != nil {
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
