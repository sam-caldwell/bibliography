package openlibrary

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

type fakeResp struct {
	status int
	body   string
}

type fakeHTTP struct{ resp fakeResp }

func (f fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	r := &http.Response{
		StatusCode: f.resp.status,
		Body:       ioutil.NopCloser(strings.NewReader(f.resp.body)),
		Header:     make(http.Header),
	}
	return r, nil
}

func TestFetchBookByISBN_Success(t *testing.T) {
	// Minimal OpenLibrary data shape for jscmd=data
	body := `{
        "ISBN:9780132350884": {
            "title": "Clean Code",
            "publish_date": "2008",
            "url": "https://openlibrary.org/books/OL12345M/Clean_Code",
            "authors": [{"name": "Robert C. Martin"}],
            "publishers": [{"name": "Prentice Hall"}],
            "subjects": [{"name": "Software engineering"}, {"name": "Programming"}]
        }
    }`
	old := client
	defer func() { client = old }()
	client = fakeHTTP{resp: fakeResp{status: 200, body: body}}

	e, err := FetchBookByISBN(context.Background(), "9780132350884")
	if err != nil {
		t.Fatalf("FetchBookByISBN: %v", err)
	}
	if e.Type != "book" || e.APA7.Title != "Clean Code" {
		t.Fatalf("bad mapping: %+v", e)
	}
	if e.APA7.Publisher == "" || e.APA7.Year == nil {
		t.Fatalf("publisher/year missing: %+v", e)
	}
	if e.APA7.URL == "" || e.APA7.Accessed == "" {
		t.Fatalf("url/accessed missing: %+v", e)
	}
	if len(e.Annotation.Keywords) == 0 || e.Annotation.Summary == "" {
		t.Fatalf("keywords/summary missing: %+v", e)
	}
}

func TestFetchBookByISBN_NoData(t *testing.T) {
	// OpenLibrary returns empty, Google returns 404
	old := client
	defer func() { client = old }()
	client = routeHTTP{routes: []route{{"openlibrary.org/api/books", 200, "{}"}, {"googleapis.com/books", 404, "not found"}}}
	if _, err := FetchBookByISBN(context.Background(), "0000"); err == nil {
		t.Fatalf("expected error for missing key and google fallback")
	}
}

func TestFetchBookByISBN_FallbackGoogle(t *testing.T) {
	// OpenLibrary empty; Google returns a volume
	old := client
	defer func() { client = old }()
	google := `{"items":[{"volumeInfo":{"title":"Clean Code","authors":["Robert C. Martin"],"publisher":"Prentice Hall","publishedDate":"2008","description":"A book.","categories":["Software Engineering"],"infoLink":"https://books.google.com/books?id=ttMsCwAAQBAJ"}}]}`
	client = routeHTTP{routes: []route{{"openlibrary.org/api/books", 200, "{}"}, {"googleapis.com/books", 200, google}}}
	e, err := FetchBookByISBN(context.Background(), "9780132350884")
	if err != nil {
		t.Fatalf("fallback google: %v", err)
	}
	if e.APA7.Title != "Clean Code" || e.APA7.Publisher != "Prentice Hall" {
		t.Fatalf("bad mapping: %+v", e)
	}
	if e.APA7.URL == "" || e.Annotation.Summary == "" {
		t.Fatalf("missing url/summary: %+v", e)
	}
	if len(e.APA7.Authors) == 0 || e.APA7.Authors[0].Family != "Martin" {
		t.Fatalf("authors parse: %+v", e.APA7.Authors)
	}
}

func TestFetchBookByISBN_Normalizes9DigitISBN(t *testing.T) {
	// OpenLibrary empty; Google returns a volume for normalized 10-digit
	old := client
	defer func() { client = old }()
	google := `{"items":[{"volumeInfo":{"title":"Some Title","authors":["Jane Roe"],"publisher":"Acme","publishedDate":"1999","description":"Desc","categories":["Cat"],"infoLink":"https://books.google.com/..."}}]}`
	client = routeHTTP{routes: []route{{"openlibrary.org/api/books", 200, "{}"}, {"isbn%3A0262060164", 200, google}}}
	e, err := FetchBookByISBN(context.Background(), "026206016")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if e.APA7.ISBN != "0262060164" {
		t.Fatalf("expected normalized ISBN-10, got %q", e.APA7.ISBN)
	}
	if len(e.APA7.Authors) == 0 || e.APA7.Authors[0].Family == "" {
		t.Fatalf("authors not parsed: %+v", e.APA7.Authors)
	}
}

func TestSplitName(t *testing.T) {
	fam, giv := splitName("Jane Q Public")
	if fam != "Public" || giv != "J. Q." {
		t.Fatalf("got %s %s", fam, giv)
	}
	fam, giv = splitName("Plato")
	if fam != "Plato" || giv != "" {
		t.Fatalf("got %s %s", fam, giv)
	}
}

func TestNormalizeISBNAndCheckDigit(t *testing.T) {
	if normalizeISBN("0-262-06016") != "0262060164" {
		t.Fatalf("normalizeISBN failed")
	}
	if isbn10CheckDigit("026206016") != "4" {
		t.Fatalf("isbn10CheckDigit failed")
	}
}

func TestFetchBookByISBN_HTTPErrors(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = routeHTTP{routes: []route{{"openlibrary.org/api/books", 500, "boom"}}}
	if _, err := FetchBookByISBN(context.Background(), "123"); err == nil {
		t.Fatalf("expected http error from OpenLibrary")
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
			return &http.Response{StatusCode: rt.status, Body: ioutil.NopCloser(strings.NewReader(rt.body)), Header: make(http.Header)}, nil
		}
	}
	return &http.Response{StatusCode: 404, Body: ioutil.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
}

func TestFetchBookByISBN_DescriptionFromDetails(t *testing.T) {
	old := client
	defer func() { client = old }()
	data := `{"ISBN:111": {"title":"T","publish_date":"2010","authors":[{"name":"Jane Doe"}],"publishers":[{"name":"P"}]}}`
	details := `{"ISBN:111": {"details": {"description": "A rich description from details.", "subjects": ["Foo", {"name":"Bar"}]}}}`
	client = routeHTTP{routes: []route{{"jscmd=data", 200, data}, {"jscmd=details", 200, details}}}
	e, err := FetchBookByISBN(context.Background(), "111")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if e.Annotation.Summary != "A rich description from details." {
		t.Fatalf("summary mismatch: %q", e.Annotation.Summary)
	}
	// details subjects should enrich keywords
	has := func(k string) bool {
		for _, s := range e.Annotation.Keywords {
			if s == k {
				return true
			}
		}
		return false
	}
	if !has("foo") || !has("bar") {
		t.Fatalf("expected enriched keywords from details: %+v", e.Annotation.Keywords)
	}
}

func TestFetchBookByISBN_DescriptionFromWork(t *testing.T) {
	old := client
	defer func() { client = old }()
	data := `{"ISBN:222": {"title":"T","publish_date":"2011","authors":[{"name":"Jane Doe"}],"publishers":[{"name":"P"}]}}`
	details := `{"ISBN:222": {"details": {"works": [{"key": "/works/OL123W"}]}}}`
	work := `{"description": {"value": "A work description."}}`
	client = routeHTTP{routes: []route{{"jscmd=data", 200, data}, {"jscmd=details", 200, details}, {"/works/OL123W.json", 200, work}}}
	e, err := FetchBookByISBN(context.Background(), "222")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if e.Annotation.Summary != "A work description." {
		t.Fatalf("summary mismatch: %q", e.Annotation.Summary)
	}
}
