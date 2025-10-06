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

func TestFetchRFC_XMLPath(t *testing.T) {
	// Minimal RFC Editor XML structure
	xml := `<?xml version="1.0"?><rfc><front>
        <title>Test RFC Title</title>
        <author fullname="John Q Public"><name><given>John Q</given><surname>Public</surname></name></author>
        <date month="May" year="2021" day="3"/>
        <seriesInfo name="RFC" value="9999"/>
        <seriesInfo name="DOI" value="10.17487/RFC9999"/>
        <abstract><t>First para.</t><t>Second para.</t></abstract>
    </front></rfc>`
	SetHTTPClient(testHTTPDoer{status: 200, body: xml})
	defer SetHTTPClient(&http.Client{})
	e, err := FetchRFC(context.Background(), "9999")
	if err != nil {
		t.Fatalf("xml path err: %v", err)
	}
	if e.APA7.Title != "Test RFC Title" || e.APA7.ContainerTitle != "RFC 9999" {
		t.Fatalf("map: %+v", e)
	}
	if e.APA7.Year == nil || *e.APA7.Year != 2021 {
		t.Fatalf("year: %+v", e.APA7.Year)
	}
	if e.APA7.DOI != "10.17487/RFC9999" {
		t.Fatalf("doi: %s", e.APA7.DOI)
	}
	if !strings.Contains(e.Annotation.Summary, "First para.") {
		t.Fatalf("summary: %q", e.Annotation.Summary)
	}
}

func TestFetchRFC_FallbackDatatrackerHTML(t *testing.T) {
	// First request: XML 404, then datatracker HTML success
	old := client
	defer func() { client = old }()
	client = routeHTTP{routes: []route{{"rfc9998.xml", 404, "not found"}, {"doc/html/rfc9998", 200, `<title>RFC 9998 - Title</title> May 2010 DOI: 10.17487/RFC9998`}}}
	e, err := FetchRFC(context.Background(), "rfc9998")
	if err != nil {
		t.Fatalf("fallback err: %v", err)
	}
	if e.APA7.Title == "" || e.APA7.DOI == "" {
		t.Fatalf("bad map: %+v", e)
	}
}

func TestFetchRFC_BibtexHTTPError(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = testHTTPDoer{status: 500, body: "boom"}
	if _, err := FetchRFC(context.Background(), "1234"); err == nil {
		t.Fatalf("expected error on bibtex http failure")
	}
}

func TestFetchRFC_BibtexParseError(t *testing.T) {
	old := client
	defer func() { client = old }()
	// Missing title field to trigger parse failure in bibtex path; should fallback to XML 404 then datatracker 404 -> error
	client = routeHTTP{routes: []route{{"/bibtex/", 200, `@misc{rfc1234, year=2000}`}, {"rfc1234.xml", 404, "not found"}, {"doc/html/rfc1234", 404, "not found"}}}
	if _, err := FetchRFC(context.Background(), "1234"); err == nil {
		t.Fatalf("expected parse error propagation")
	}
}

func TestHelpers_MonthAndNormalize(t *testing.T) {
	if monthToNum("Feb") != 2 || monthToNum("September") != 9 {
		t.Fatalf("monthToNum failed")
	}
	if normalizeRFCNumber("RFC 5424 ") != "5424" {
		t.Fatalf("normalizeRFCNumber failed")
	}
	fam, giv := splitFullName("John Q Public")
	if fam != "Public" || giv == "" {
		t.Fatalf("splitFullName")
	}
	if sanitizeAbstract(" A\n\n B \n\n\n C ") != "A\n\nB\n\nC" {
		t.Fatalf("sanitizeAbstract")
	}
}

func TestBibtexParsingHelpers(t *testing.T) {
	s := `@misc{rfc9997,
  title = {A Title},
  author = {Doe, Jane and John Smith},
  year = 2012,
  month = oct,
  pages = {1--10},
}`
	fields := parseBibtexFields(s)
	if fields["title"] != "A Title" {
		t.Fatalf("title parse: %+v", fields)
	}
	if getBibField(s, "year") != "2012" || getBibField(s, "month") != "oct" {
		t.Fatalf("getBibField")
	}
	block := extractBibtexBlock("xxx\n" + s + "\nmore")
	if !strings.HasPrefix(block, "@misc") {
		t.Fatalf("extract block: %q", block)
	}
	fam, giv := splitAuthorNameBib("Doe, Jane Q")
	if fam != "Doe" || giv != "Jane Q" {
		t.Fatalf("splitAuthorNameBib comma")
	}
	fam, giv = splitAuthorNameBib("Jane Q Public")
	if fam != "Public" || giv != "Jane Q" {
		t.Fatalf("splitAuthorNameBib space")
	}
	if normalizeSpaces(" A   B \n C ") != "A B C" {
		t.Fatalf("normalizeSpaces")
	}
}

type route struct {
	match  string
	status int
	body   string
}
type routeHTTP struct{ routes []route }

func (r routeHTTP) Do(req *http.Request) (*http.Response, error) {
	u := req.URL.String()
	for _, rt := range r.routes {
		if strings.Contains(u, rt.match) {
			return &http.Response{StatusCode: rt.status, Body: io.NopCloser(strings.NewReader(rt.body)), Header: make(http.Header)}, nil
		}
	}
	return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
}
