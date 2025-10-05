package doi

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

type testHTTP struct {
	status int
	body   string
}

func (t testHTTP) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.status, Body: io.NopCloser(strings.NewReader(t.body)), Header: make(http.Header)}, nil
}

func TestFetchArticleByDOI_Success(t *testing.T) {
	csl := `{
        "title": "A Sample Article",
        "author": [{"family":"Doe","given":"Jane Q"},{"family":"Smith","given":"John"}],
        "container-title": "Journal of Things",
        "issued": {"date-parts": [[2023,7,14]]},
        "DOI": "10.1234/sample",
        "volume": "10",
        "issue": "2",
        "page": "10-20",
        "publisher": "ACME"
    }`
	old := client
	SetHTTPClient(testHTTP{status: 200, body: csl})
	defer SetHTTPClient(old)

	e, err := FetchArticleByDOI(context.Background(), "10.1234/sample")
	if err != nil {
		t.Fatalf("FetchArticleByDOI: %v", err)
	}
	if e.Type != "article" || e.APA7.Title != "A Sample Article" {
		t.Fatalf("bad mapping: %+v", e)
	}
	if e.APA7.Journal != "Journal of Things" {
		t.Fatalf("journal mismatch: %q", e.APA7.Journal)
	}
	if e.APA7.Volume != "10" || e.APA7.Issue != "2" || e.APA7.Pages != "10-20" {
		t.Fatalf("vol/issue/pages: %+v", e.APA7)
	}
	if e.APA7.DOI != "10.1234/sample" {
		t.Fatalf("doi mismatch: %q", e.APA7.DOI)
	}
	if e.APA7.URL != "https://doi.org/10.1234/sample" {
		t.Fatalf("url mismatch: %q", e.APA7.URL)
	}
	if e.APA7.Year == nil || *e.APA7.Year != 2023 || e.APA7.Date != "2023-07-14" {
		t.Fatalf("date/year mismatch: %+v", e.APA7)
	}
	if len(e.APA7.Authors) == 0 || e.APA7.Authors[0].Family != "Doe" || e.APA7.Authors[0].Given != "J. Q." {
		t.Fatalf("authors: %+v", e.APA7.Authors)
	}
	if e.Annotation.Summary == "" {
		t.Fatalf("expected non-empty summary")
	}
	if len(e.Annotation.Keywords) == 0 {
		t.Fatalf("expected default keywords")
	}
	match, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, e.APA7.Accessed)
	if !match {
		t.Fatalf("accessed not set as date: %q", e.APA7.Accessed)
	}
}

func TestFetchArticleByDOI_HTTPError(t *testing.T) {
	old := client
	SetHTTPClient(testHTTP{status: 404, body: "not found"})
	defer SetHTTPClient(old)
	if _, err := FetchArticleByDOI(context.Background(), "10.0/none"); err == nil {
		t.Fatalf("expected error on http failure")
	}
}
