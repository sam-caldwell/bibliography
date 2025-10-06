package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"bibliography/src/internal/doi"
	"bibliography/src/internal/openlibrary"
	rfcpkg "bibliography/src/internal/rfc"
	"bibliography/src/internal/schema"
	"bibliography/src/internal/summarize"
	webfetch "bibliography/src/internal/webfetch"
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

	// Stub commit for index to avoid requiring a git repo
	commitAndPush = func(paths []string, msg string) error { return nil }
	t.Cleanup(func() { commitAndPush = nil })

	// Seed two entries
	if err := os.MkdirAll("data/citations", 0o755); err != nil {
		t.Fatal(err)
	}
	e1 := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "A", URL: "https://a", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"golang", "apa7"}}}
	e2 := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "B"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"golang", "yaml"}}}
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
	if !bytes.Contains([]byte(out), []byte("data/citations/site/")) || !bytes.Contains([]byte(out), []byte(": A")) {
		t.Fatalf("expected site match for title A, got: %q", out)
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
	rootCmd.AddCommand(newAddCmd())

	// Run add site
	if _, err := execCmd(rootCmd, "add", "site", "https://example.com"); err != nil {
		t.Fatalf("add: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "site"))
	if len(files) != 1 || !strings.HasSuffix(files[0].Name(), ".yaml") {
		t.Fatalf("expected one yaml in site dir, got %v", files)
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
	rootCmd.AddCommand(newAddCmd())

	if _, err := execCmd(rootCmd, "add", "book", "--name", "The Book"); err != nil {
		t.Fatalf("add book: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "books"))
	if len(files) != 1 {
		t.Fatalf("expected one book yaml written")
	}

	if _, err := execCmd(rootCmd, "add", "movie", "Best", "Movie", "--date", "2024-01-01"); err != nil {
		t.Fatalf("add movie: %v", err)
	}
	files, _ = os.ReadDir(filepath.Join("data/citations", "movie"))
	if len(files) != 1 {
		t.Fatalf("expected one movie yaml written")
	}
}

func intPtr(v int) *int { return &v }

// Removed: requirement for OPENAI_API_KEY

func TestIndexPrintsPath(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Stub commit for index
	commitAndPush = func(paths []string, msg string) error { return nil }
	t.Cleanup(func() { commitAndPush = nil })

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
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "site", "https://t"); err == nil {
		t.Fatalf("expected commit error to surface")
	}

	// Now simulate success
	commitAndPush = func(paths []string, msg string) error { return nil }
	if _, err := execCmd(rootCmd, "add", "site", "https://t"); err != nil {
		t.Fatalf("add site: %v", err)
	}
	// Verify at least one YAML has accessed set
	entries, _ := os.ReadDir(filepath.Join("data/citations", "site"))
	if len(entries) == 0 {
		t.Fatalf("expected some entries in site dir")
	}
	var b []byte
	var err error
	for _, de := range entries {
		bb, rerr := os.ReadFile(filepath.Join("data/citations", "site", de.Name()))
		if rerr == nil && bytes.Contains(bb, []byte("accessed:")) {
			b = bb
			break
		}
	}
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
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "article", "--doi", "10.1234/x"); err != nil {
		t.Fatalf("add article: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "article"))
	if len(files) != 1 {
		t.Fatalf("expected article yaml written")
	}
	// verify DOI and doi.org URL recorded in YAML
	b, err := os.ReadFile(filepath.Join("data/citations", "article", files[0].Name()))
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
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "article", "--doi", "10.5555/abc"); err != nil {
		t.Fatalf("add article: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "article"))
	if len(files) != 1 {
		t.Fatalf("expected article yaml written")
	}
	by, err := os.ReadFile(filepath.Join("data/citations", "article", files[0].Name()))
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

func TestLookupArticleByURL_OGTags(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// stub web page with OG/meta and JSON-LD
	html := `<!doctype html><html><head>
    <meta property="og:title" content="Faster-Than-Light Telegraph That Wasn't">
    <meta property="og:site_name" content="Scientific American">
    <meta property="article:published_time" content="2024-05-01T12:00:00Z">
    <meta name="author" content="Jane Doe">
    <meta name="description" content="A piece about a telegraph mistake.">
    <script type="application/ld+json">{"@type":"NewsArticle","headline":"Faster-Than-Light Telegraph That Wasn't","author":{"name":"Jane Doe"},"datePublished":"2024-05-01","publisher":{"name":"Scientific American"}}</script>
    <title>FTL Telegraph</title>
    </head><body>...</body></html>`

	// make a minimal HTTPDoer that returns this HTML for any URL
	webfetch.SetHTTPClient(testHTTPDoer{status: 200, body: html})
	t.Cleanup(func() { webfetch.SetHTTPClient(&http.Client{}) })

	commitAndPush = func(paths []string, msg string) error { return nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "article", "--url", "https://example.com/post"); err != nil {
		t.Fatalf("add article by url: %v", err)
	}
	// slug from title + year
	files, _ := os.ReadDir(filepath.Join("data/citations", "article"))
	if len(files) != 1 {
		t.Fatalf("expected YAML written")
	}
	by, err := os.ReadFile(filepath.Join("data/citations", "article", files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(by, []byte("Scientific American")) {
		t.Fatalf("expected container/publisher in yaml: %s", string(by))
	}
	if !bytes.Contains(by, []byte("accessed:")) {
		t.Fatalf("expected accessed date")
	}
}

func TestLookupArticleByURL_PDF(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// Minimal PDF-like content containing metadata
	pdf := "%PDF-1.4\n1 0 obj\n<< /Title (Sample PDF Title) /Author (John Q Public) /CreationDate (D:20230501120000Z) >>\nendobj\n"
	// Set content-type via a custom doer that sets header
	webfetch.SetHTTPClient(struct{ testHTTPDoer }{testHTTPDoer{status: 200, body: pdf}})
	t.Cleanup(func() { webfetch.SetHTTPClient(&http.Client{}) })

	commitAndPush = func(paths []string, msg string) error { return nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "article", "--url", "https://example.com/sample.pdf"); err != nil {
		t.Fatalf("add article by pdf url: %v", err)
	}
	// slug from title + year
	files, _ := os.ReadDir(filepath.Join("data/citations", "article"))
	if len(files) != 1 {
		t.Fatalf("expected YAML written")
	}
	by, err := os.ReadFile(filepath.Join("data/citations", "article", files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(by, []byte("John Q")) && !bytes.Contains(by, []byte("Public")) {
		t.Fatalf("expected author in YAML: %s", string(by))
	}
}

func TestLookupRFC_Basic(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// stub rfc HTTP (XML)
	xml := `<?xml version="1.0"?><rfc><front>
        <title>The Syslog Protocol</title>
        <author fullname="Rainer Gerhards"><name><given>Rainer</given><surname>Gerhards</surname></name></author>
        <date month="Mar" year="2009"/>
        <seriesInfo name="RFC" value="5424"/>
        <seriesInfo name="DOI" value="10.17487/RFC5424"/>
        <abstract><t>This document describes the syslog protocol.</t></abstract>
    </front></rfc>`
	rfcpkg.SetHTTPClient(testHTTPDoer{status: 200, body: xml})
	t.Cleanup(func() { rfcpkg.SetHTTPClient(&http.Client{}) })

	commitAndPush = func(paths []string, msg string) error { return nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "rfc", "rfc5424"); err != nil {
		t.Fatalf("add rfc: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "rfc"))
	if len(files) != 1 {
		t.Fatalf("expected rfc yaml written")
	}
	by, err := os.ReadFile(filepath.Join("data/citations", "rfc", files[0].Name()))
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
	id := schema.NewID()
	yaml := "" +
		"id: " + id + "\n" +
		"type: article\n" +
		"apa7:\n" +
		"  title: T\n" +
		"  url: https://dl.acm.org/doi/10.1145/12345.67890\n" +
		"  accessed: \"2025-01-01\"\n" +
		"annotation:\n" +
		"  summary: s\n" +
		"  keywords: [k]\n"
	if err := os.WriteFile(filepath.Join("data/citations/article", id+".yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run repair-doi
	commitAndPush = func(paths []string, msg string) error { return nil }
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newRepairDOICmd())
	if _, err := execCmd(rootCmd, "repair-doi"); err != nil {
		t.Fatalf("repair-doi: %v", err)
	}
	b, err := os.ReadFile(filepath.Join("data/citations/article", id+".yaml"))
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
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "article", "--title", "X", "--author", "Doe, J.", "--journal", "J", "--date", "2023-01-01"); err != nil {
		t.Fatalf("add article by metadata: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "article"))
	if len(files) != 1 {
		t.Fatalf("expected one article yaml written")
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
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "book", "--name", "Name", "--author", "Smith, J.", "--isbn", "123", "--keywords", "k1,k2"); err != nil {
		t.Fatalf("add book by isbn: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "books"))
	if len(files) != 1 {
		t.Fatalf("expected book yaml written")
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
	rootCmd.AddCommand(newAddCmd())
	// Provide empty stdin so manualAdd immediately hits EOF and errors
	rootCmd.SetIn(strings.NewReader(""))
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
	rootCmd.AddCommand(newAddCmd())
	if _, err := execCmd(rootCmd, "add", "book", "--name", "Hello World"); err != nil {
		t.Fatalf("add book: %v", err)
	}
	files, _ := os.ReadDir(filepath.Join("data/citations", "books"))
	if len(files) != 1 {
		t.Fatalf("expected one book yaml written")
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
	if err := doAdd(context.Background(), "website", map[string]string{"url": "https://x", "title": "T"}); err == nil {
		t.Fatalf("expected error when write entry fails")
	}
}

// Compile-time ensure doAdd uses context, no-op just to silence unused in coverage.
var _ = context.Background()

// --- Summarize command tests ---

type oaFakeDoer struct{ body string }

func (f oaFakeDoer) Do(req *http.Request) (*http.Response, error) {
	if f.body == "" {
		f.body = `{"choices":[{"message":{"content":"[\"alpha\",\"beta\"]"}}]}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(f.body)), Header: make(http.Header)}, nil
}

func TestSummarizeCommand_UpdatesSummaryAndKeywords(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Start an HTTP server to be considered accessible
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()

	// Seed a citation with boilerplate summary and URL
	if err := os.MkdirAll("data/citations/article", 0o755); err != nil {
		t.Fatal(err)
	}
	id := schema.NewID()
	y := "" +
		"id: " + id + "\n" +
		"type: article\n" +
		"apa7:\n  title: Test Title\n  url: \"" + srv.URL + "\"\n  accessed: \"2025-01-01\"\n" +
		"annotation:\n  summary: Bibliographic record for x.\n  keywords: [k]\n"
	if err := os.WriteFile("data/citations/article/"+id+".yaml", []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}

	// Stub OpenAI client for both summary and keywords
	t.Setenv("OPENAI_API_KEY", "x")
	summarize.SetHTTPClient(oaFakeDoer{body: `{"choices":[{"message":{"content":"[\"alpha\",\"beta\"]"}}]}`})
	t.Cleanup(func() { summarize.SetHTTPClient(&http.Client{}) })

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newSummarizeCmd())
	out, err := execCmd(rootCmd, "summarize")
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if !strings.Contains(out, "updated data/citations/article/") {
		t.Fatalf("expected updated notice, got %q", out)
	}
	files, _ := os.ReadDir("data/citations/article")
	if len(files) != 1 {
		t.Fatalf("expected one article file")
	}
	b, _ := os.ReadFile(filepath.Join("data/citations/article", files[0].Name()))
	if !bytes.Contains(b, []byte("summary:")) {
		t.Fatalf("summary not updated: %s", string(b))
	}
	if !bytes.Contains(b, []byte("keywords:")) || !bytes.Contains(b, []byte("alpha")) {
		t.Fatalf("keywords not updated: %s", string(b))
	}
}

func TestSummarizeCommand_SkipsWhenNotAccessible(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	if err := os.MkdirAll("data/citations/article", 0o755); err != nil {
		t.Fatal(err)
	}
	id2 := schema.NewID()
	y := "" +
		"id: " + id2 + "\n" +
		"type: article\n" +
		"apa7:\n  title: Test\n  url: https://127.0.0.1:9/\n  accessed: \"2025-01-01\"\n" +
		"annotation:\n  summary: Bibliographic record for x.\n  keywords: [k]\n"
	if err := os.WriteFile("data/citations/article/"+id2+".yaml", []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "x")
	summarize.SetHTTPClient(oaFakeDoer{})
	t.Cleanup(func() { summarize.SetHTTPClient(&http.Client{}) })
	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newSummarizeCmd())
	out, err := execCmd(rootCmd, "summarize")
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if !strings.Contains(out, "no entries needed summaries") && !strings.Contains(out, "skip") {
		t.Fatalf("expected skip or none message, got %q", out)
	}
}

// migrate command removed

// --- Edit command tests ---

func TestEdit_ShowPrintsYAML(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	// seed minimal valid YAML
	id := schema.NewID()
	if err := os.MkdirAll(filepath.Join("data", "citations", "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	y := "" +
		"id: " + id + "\n" +
		"type: website\n" +
		"apa7:\n  title: Title\n  url: https://x\n  accessed: \"2025-01-01\"\n" +
		"annotation:\n  summary: s\n  keywords: [k]\n"
	if err := os.WriteFile(filepath.Join("data/citations/site", id+".yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newEditCmd())
	out, err := execCmd(rootCmd, "edit", "--id", id)
	if err != nil {
		t.Fatalf("edit show: %v", err)
	}
	if !strings.Contains(out, "id: "+id) || !strings.Contains(out, "apa7:") {
		t.Fatalf("expected YAML printed, got: %q", out)
	}
}

func TestEdit_UpdateScalarAndArray(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	id := schema.NewID()
	if err := os.MkdirAll(filepath.Join("data", "citations", "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	y := "" +
		"id: " + id + "\n" +
		"type: website\n" +
		"apa7:\n  title: Old\n  url: https://x\n  accessed: \"2025-01-01\"\n" +
		"annotation:\n  summary: s\n  keywords: [k]\n"
	path := filepath.Join("data/citations/site", id+".yaml")
	if err := os.WriteFile(path, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newEditCmd())
	out, err := execCmd(rootCmd, "edit", "--id", id, "--apa7.title=New Title", `--annotation.keywords=["alpha","beta"]`)
	if err != nil {
		t.Fatalf("edit update: %v", err)
	}
	if !strings.Contains(out, "updated data/citations/site/") {
		t.Fatalf("expected updated message, got %q", out)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("New Title")) {
		t.Fatalf("expected title updated, got:\n%s", string(b))
	}
	if !bytes.Contains(b, []byte("alpha")) || !bytes.Contains(b, []byte("beta")) {
		t.Fatalf("expected keywords updated, got:\n%s", string(b))
	}
}

func TestEdit_TypeChangeMovesFile(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	id := schema.NewID()
	if err := os.MkdirAll(filepath.Join("data", "citations", "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	y := "" +
		"id: " + id + "\n" +
		"type: website\n" +
		"apa7:\n  title: Title\n  url: https://x\n  accessed: \"2025-01-01\"\n" +
		"annotation:\n  summary: s\n  keywords: [k]\n"
	oldPath := filepath.Join("data/citations/site", id+".yaml")
	if err := os.WriteFile(oldPath, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newEditCmd())
	out, err := execCmd(rootCmd, "edit", "--id", id, "--type=book")
	if err != nil {
		t.Fatalf("edit change type: %v", err)
	}
	if !strings.Contains(out, "moved data/citations/site/") || !strings.Contains(out, "data/citations/books/") {
		t.Fatalf("expected move notice, got %q", out)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path removed")
	}
	// new file exists
	matches, _ := filepath.Glob(filepath.Join("data/citations/books", id+".yaml"))
	if len(matches) != 1 {
		t.Fatalf("expected new file in books dir")
	}
}

func TestEdit_SetURLSetsAccessed(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)

	id := schema.NewID()
	if err := os.MkdirAll(filepath.Join("data", "citations", "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	// URL initially empty, accessed missing
	y := "" +
		"id: " + id + "\n" +
		"type: website\n" +
		"apa7:\n  title: Title\n" +
		"annotation:\n  summary: s\n  keywords: [k]\n"
	p := filepath.Join("data/citations/site", id+".yaml")
	if err := os.WriteFile(p, []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}

	rootCmd = &cobra.Command{Use: "bib"}
	rootCmd.AddCommand(newEditCmd())
	if _, err := execCmd(rootCmd, "edit", "--id", id, "--apa7.url=https://example.com"); err != nil {
		t.Fatalf("edit set url: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("accessed:")) {
		t.Fatalf("expected accessed set when url provided, got:\n%s", string(b))
	}
}
