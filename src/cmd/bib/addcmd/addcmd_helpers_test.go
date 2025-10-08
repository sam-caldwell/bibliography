package addcmd

import (
	"bibliography/src/internal/schema"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelperFunctions_ApplyAndDerive(t *testing.T) {
	e := schema.Entry{Type: "article"}
	// journal
	applyJournal(&e, map[string]string{"journal": "J"})
	if e.APA7.Journal != "J" || e.APA7.ContainerTitle != "J" {
		t.Fatalf("journal/container not applied")
	}
	// date/year
	applyDate(&e, map[string]string{"date": "2020-04-03"})
	if e.APA7.Date != "2020-04-03" || e.APA7.Year == nil || *e.APA7.Year != 2020 {
		t.Fatalf("date/year not applied")
	}
	// url
	applyURL(&e, map[string]string{"url": "https://x"})
	if e.APA7.URL != "https://x" {
		t.Fatalf("url not applied")
	}
	// author hint
	applyAuthorHint(&e, map[string]string{"author": "Doe, J"})
	if len(e.APA7.Authors) == 0 || e.APA7.Authors[0].Family != "Doe" {
		t.Fatalf("author not parsed")
	}
	// ids
	applyIDs(&e, map[string]string{"isbn": "123", "doi": "10.1/x"})
	if e.APA7.ISBN != "123" || e.APA7.DOI != "10.1/x" || e.APA7.URL == "" {
		t.Fatalf("ids not applied or doi url not set")
	}
	// defaults
	applyDefaults(&e, "article", []string{"x"})
	if e.ID == "" || len(e.Annotation.Keywords) == 0 {
		t.Fatalf("defaults not applied")
	}
	// summary
	e.APA7.Title = "Example"
	applyManualSummary(&e)
	if e.Annotation.Summary == "" {
		t.Fatalf("summary not set")
	}
	// deriveTitle for website and patent from URL host
	if title, _ := deriveTitle("website", map[string]string{"url": "https://example.com/x"}); title != "example.com" {
		t.Fatalf("deriveTitle website from host failed: %q", title)
	}
	if title, _ := deriveTitle("patent", map[string]string{"url": "https://patents.example.com/x"}); title == "" {
		t.Fatalf("deriveTitle patent from url failed")
	}
}

func TestParseAuthorAndSplitAuthorsBySemi(t *testing.T) {
	fam, giv := parseAuthor("Doe, Jane")
	if fam != "Doe" || giv == "" {
		t.Fatalf("parseAuthor failed")
	}
	fam2, giv2 := parseAuthor("Org Name")
	if fam2 != "Org Name" || giv2 != "" {
		t.Fatalf("parseAuthor org failed")
	}
	names := splitAuthorsBySemi("A; B ; ; C")
	if len(names) != 3 {
		t.Fatalf("splitAuthorsBySemi failed: %+v", names)
	}
}

func TestManualAdd_TitleRequired(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Provide empty title as first prompt answer
	in := strings.NewReader("\n")
	out := new(strings.Builder)
	cmd := &cobra.Command{}
	cmd.SetIn(in)
	cmd.SetOut(out)
	err := manualAdd(cmd, func(paths []string, msg string) error { return nil }, "article", nil)
	if err == nil {
		t.Fatalf("expected error when title is empty")
	}
}

func TestDeriveTitle_ErrorForNonURLTypes(t *testing.T) {
	if _, err := deriveTitle("article", map[string]string{}); err == nil {
		t.Fatalf("expected error for missing title on non-url types")
	}
}

func TestManualAdd_BookFlow(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Inputs for prompts in order for type book:
	// Title, Authors, Date, URL, Publisher, ISBN, Summary, Keywords
	in := strings.NewReader(strings.Join([]string{
		"My Book",         // title
		"Doe, Jane; Poe",  // authors
		"2020-01-01",      // date
		"https://example", // url
		"Pub",             // publisher
		"1234567890",      // isbn
		"A summary",       // summary
		"book, test",      // keywords
		"",
	}, "\n"))
	out := new(strings.Builder)
	cmd := &cobra.Command{}
	cmd.SetIn(in)
	cmd.SetOut(out)
	// commit stub
	commit := func(paths []string, msg string) error { return nil }
	if err := manualAdd(cmd, commit, "book", nil); err != nil {
		t.Fatalf("manualAdd book: %v", err)
	}
	// verify file written under books
	files, _ := os.ReadDir(filepath.Join("data", "citations", "books"))
	if len(files) != 1 {
		t.Fatalf("expected one book yaml written, got %d", len(files))
	}
}
