package rfc

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testHTTPDoer struct {
	status int
	body   string
}

func (t testHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.status, Body: io.NopCloser(strings.NewReader(t.body)), Header: make(http.Header)}, nil
}

func TestFetchRFC_ParsesTitleDateAuthorsDOI(t *testing.T) {
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
	SetHTTPClient(testHTTPDoer{status: 200, body: bib})
	defer SetHTTPClient(&http.Client{})

	e, err := FetchRFC(context.Background(), "rfc5424")
	if err != nil {
		t.Fatalf("FetchRFC: %v", err)
	}
	if e.Type != "rfc" {
		t.Fatalf("type: %s", e.Type)
	}
	if e.ID != "rfc5424" {
		t.Fatalf("id: %s", e.ID)
	}
	if e.APA7.Title != "The Syslog Protocol" {
		t.Fatalf("title: %s", e.APA7.Title)
	}
	if e.APA7.ContainerTitle != "RFC 5424" {
		t.Fatalf("container: %s", e.APA7.ContainerTitle)
	}
	if e.APA7.Year == nil || *e.APA7.Year != 2009 {
		t.Fatalf("year: %v", e.APA7.Year)
	}
	if e.APA7.DOI != "10.17487/RFC5424" {
		t.Fatalf("doi: %s", e.APA7.DOI)
	}
	if e.APA7.URL == "" {
		t.Fatalf("url missing")
	}
	if len(e.APA7.Authors) == 0 || e.APA7.Authors[0].Family != "Gerhards" {
		t.Fatalf("authors not parsed correctly: %+v", e.APA7.Authors)
	}
	if e.APA7.BibTeXURL == "" {
		t.Fatalf("expected BibTeXURL to be set")
	}
	if !strings.Contains(e.Annotation.Summary, "syslog protocol") {
		t.Fatalf("expected abstract in summary, got: %q", e.Annotation.Summary)
	}
}
