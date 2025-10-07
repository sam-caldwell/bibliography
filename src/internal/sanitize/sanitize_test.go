package sanitize

import (
    "net/url"
    "testing"
    "unicode/utf8"

    "bibliography/src/internal/schema"
)

func TestCleanString(t *testing.T) {
    in := "  \tHello\x00World\n  "
    out := CleanString(in, 100)
    if out != "HelloWorld" && out != "HelloWorld\n" { // allow newline if preserved
        t.Fatalf("CleanString unexpected: %q", out)
    }
    if s := CleanString("abcdef", 3); s != "abc" {
        t.Fatalf("CleanString truncation: want 'abc', got %q", s)
    }
    if !utf8.ValidString(out) {
        t.Fatalf("CleanString produced invalid utf8")
    }
}

func TestCleanURL(t *testing.T) {
    if CleanURL("") != "" { t.Fatalf("CleanURL empty should be empty") }
    if CleanURL("not a url") != "" { t.Fatalf("CleanURL invalid should be empty") }
    u := CleanURL("https://example.com/a b")
    if _, err := url.Parse(u); err != nil { t.Fatalf("CleanURL not parseable: %v", err) }
    if CleanURL("ftp://x") != "" { t.Fatalf("only http/https allowed") }
}

func TestCleanKeywords(t *testing.T) {
    in := []string{" A ", "a", "B"}
    out := CleanKeywords(in)
    if len(out) != 2 { t.Fatalf("CleanKeywords dedupe failed: %v", out) }
    if out[0] != "a" && out[1] != "a" { t.Fatalf("expected 'a' present: %v", out) }
}

func TestCleanAuthorsAndEntry(t *testing.T) {
    authors := schema.Authors{{Family: " ", Given: " "}, {Family: "Doe", Given: "J."}}
    ca := CleanAuthors(authors)
    if len(ca) != 1 || ca[0].Family != "Doe" { t.Fatalf("CleanAuthors failed: %+v", ca) }

    e := schema.Entry{ID: " id ", Type: " article ", APA7: schema.APA7{Title: "  Title  ", URL: "https://e x", Accessed: " ", DOI: " 10.1/ABC ", Authors: authors}, Annotation: schema.Annotation{Summary: "  s  ", Keywords: []string{"X", "x"}}}
    CleanEntry(&e)
    if e.ID == " id " || e.Type != "article" { t.Fatalf("CleanEntry did not trim/type: %+v", e) }
    if e.APA7.URL == "https://e x" { t.Fatalf("CleanEntry URL not cleaned: %q", e.APA7.URL) }
    if len(e.Annotation.Keywords) != 1 || e.Annotation.Keywords[0] != "x" { t.Fatalf("CleanEntry keywords: %v", e.Annotation.Keywords) }
    if len(e.APA7.Authors) != 1 || e.APA7.Authors[0].Family != "Doe" { t.Fatalf("CleanEntry authors: %+v", e.APA7.Authors) }
}

