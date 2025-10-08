package booksearch

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"bibliography/src/internal/openlibrary"
)

// fakeDoer implements httpx.Doer for deterministic responses.
type fakeDoer struct {
	handler func(req *http.Request) *http.Response
}

func (f fakeDoer) Do(req *http.Request) (*http.Response, error) { return f.handler(req), nil }

func jsonResp(code int, v any) *http.Response {
	b, _ := json.Marshal(v)
	return &http.Response{StatusCode: code, Body: ioutil.NopCloser(bytes.NewReader(b)), Header: http.Header{"Content-Type": {"application/json"}}}
}
func textResp(code int, s string) *http.Response {
	return &http.Response{StatusCode: code, Body: ioutil.NopCloser(strings.NewReader(s)), Header: http.Header{"Content-Type": {"text/plain"}}}
}

func TestLookupBookByISBN_OpenLibraryFirst(t *testing.T) {
	// Stub OpenLibrary success
	openlibrary.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "openlibrary.org") && strings.Contains(req.URL.RawQuery, "jscmd=data") {
			payload := map[string]any{
				"ISBN:111": map[string]any{
					"title":        "OL Title",
					"publish_date": "2001",
					"publishers":   []map[string]string{{"name": "Pub"}},
					"authors":      []map[string]string{{"name": "Doe, Jane"}},
					"url":          "https://openlibrary.org/works/OL1W",
				},
			}
			return jsonResp(200, payload)
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { openlibrary.SetHTTPClient(&http.Client{}) })

	e, prov, attempts, err := LookupBookByISBN(context.Background(), "111")
	if err != nil {
		t.Fatalf("openlibrary path: %v", err)
	}
	if e.APA7.Title != "OL Title" || e.APA7.Publisher != "Pub" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if prov != "openlibrary" {
		t.Fatalf("provider mismatch: %s", prov)
	}
	if len(attempts) == 0 || attempts[0].Provider != "openlibrary" || !attempts[0].Success {
		t.Fatalf("attempts missing or incorrect: %+v", attempts)
	}
}

func TestLookupBookByISBN_CrossrefFallback(t *testing.T) {
	// Force OpenLibrary to return no data and Google fallback to fail
	openlibrary.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "openlibrary.org") && strings.Contains(req.URL.RawQuery, "jscmd=data") {
			// Empty map => no data
			return jsonResp(200, map[string]any{"ISBN:222": map[string]any{}})
		}
		if strings.Contains(req.URL.Host, "googleapis.com") { // Google Books fallback
			return textResp(404, "nope")
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { openlibrary.SetHTTPClient(&http.Client{}) })

	// Crossref success
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "api.crossref.org") {
			return jsonResp(200, map[string]any{
				"message": map[string]any{
					"items": []map[string]any{{
						"title":     []string{"CR Title"},
						"publisher": "CR Pub",
						"issued":    map[string]any{"date-parts": [][]int{{2019, 1, 2}}},
						"author":    []map[string]string{{"family": "Doe", "given": "J"}},
						"DOI":       "10.x/x",
						"URL":       "https://dx.doi.org/10.x/x",
					}},
				},
			})
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { SetHTTPClient(&http.Client{}) })

	e, prov, attempts, err := LookupBookByISBN(context.Background(), "222")
	if err != nil {
		t.Fatalf("crossref path: %v", err)
	}
	if e.APA7.Title != "CR Title" || e.APA7.Publisher != "CR Pub" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if prov != "crossref" {
		t.Fatalf("provider mismatch: %s", prov)
	}
	if len(attempts) < 2 || attempts[0].Provider != "openlibrary" || attempts[0].Success || attempts[1].Provider != "crossref" || !attempts[1].Success {
		t.Fatalf("attempt trace wrong: %+v", attempts)
	}
}

func TestLookupBookByISBN_OCLCFallback(t *testing.T) {
	// Make OL + Crossref fail
	openlibrary.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response { return textResp(404, "") }})
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "api.crossref.org") {
			return textResp(500, "err")
		}
		if strings.Contains(req.URL.Host, "classify.oclc.org") {
			xml := `<?xml version="1.0"?><classify><works>
                <work title="OCLC Title" author="Smith, J" hyr="2010"/>
            </works></classify>`
			return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader(xml)), Header: http.Header{"Content-Type": {"application/xml"}}}
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { openlibrary.SetHTTPClient(&http.Client{}); SetHTTPClient(&http.Client{}) })

	e, prov, attempts, err := LookupBookByISBN(context.Background(), "333")
	if err != nil {
		t.Fatalf("oclc path: %v", err)
	}
	if e.APA7.Title != "OCLC Title" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if prov != "oclc" {
		t.Fatalf("provider mismatch: %s", prov)
	}
	if len(attempts) < 3 || attempts[2].Provider != "oclc" || !attempts[2].Success {
		t.Fatalf("attempt trace wrong: %+v", attempts)
	}
}

func TestLookupBookByISBN_BNB_OpenBD_LoC_Then_OpenAI(t *testing.T) {
	// Fail OL, Crossref, OCLC; succeed at BNB
	openlibrary.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response { return textResp(404, "") }})
	// Track which branch was hit
	hit := make(map[string]bool)
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		switch {
		case strings.Contains(req.URL.Host, "api.crossref.org"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "classify.oclc.org"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "bnb.data.bl.uk"):
			hit["bnb"] = true
			return jsonResp(200, map[string]any{"results": map[string]any{
				"bindings": []map[string]map[string]string{{
					"title":         {"value": "BNB Title"},
					"publisherName": {"value": "BNB Pub"},
					"date":          {"value": "2005"},
				}},
			}})
		default:
			return textResp(404, "")
		}
	}})
	t.Cleanup(func() { openlibrary.SetHTTPClient(&http.Client{}); SetHTTPClient(&http.Client{}) })

	if e, prov, attempts, err := LookupBookByISBN(context.Background(), "444"); err != nil {
		t.Fatalf("bnb path: %v", err)
	} else if e.APA7.Title != "BNB Title" || e.APA7.Publisher != "BNB Pub" {
		t.Fatalf("unexpected: %+v", e)
	} else if prov != "bnb" {
		t.Fatalf("prov: %s", prov)
	} else if attempts[len(attempts)-1].Provider != "bnb" || !attempts[len(attempts)-1].Success {
		t.Fatalf("attempts: %+v", attempts)
	}

	// Now fail BNB too; hit openBD
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		switch {
		case strings.Contains(req.URL.Host, "api.crossref.org"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "classify.oclc.org"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "bnb.data.bl.uk"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "api.openbd.jp"):
			hit["openbd"] = true
			return jsonResp(200, []any{map[string]any{"summary": map[string]any{
				"title": "OBD Title", "publisher": "OBD Pub", "pubdate": "2012", "author": "Roe, J",
			}}})
		default:
			return textResp(404, "")
		}
	}})
	if e, prov, attempts, err := LookupBookByISBN(context.Background(), "555"); err != nil {
		t.Fatalf("openbd path: %v", err)
	} else if e.APA7.Title != "OBD Title" || e.APA7.Publisher != "OBD Pub" {
		t.Fatalf("unexpected: %+v", e)
	} else if prov != "openbd" {
		t.Fatalf("prov: %s", prov)
	} else if attempts[len(attempts)-1].Provider != "openbd" || !attempts[len(attempts)-1].Success {
		t.Fatalf("attempts: %+v", attempts)
	}

	// Fail openBD; hit LoC
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		switch {
		case strings.Contains(req.URL.Host, "api.crossref.org"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "classify.oclc.org"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "bnb.data.bl.uk"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "api.openbd.jp"):
			return textResp(500, "err")
		case strings.Contains(req.URL.Host, "loc.gov"):
			return jsonResp(200, map[string]any{"results": []map[string]any{{"title": "LoC Title", "date": "1999", "url": "https://loc.gov/x"}}})
		default:
			return textResp(404, "")
		}
	}})
	if e, prov, attempts, err := LookupBookByISBN(context.Background(), "666"); err != nil {
		t.Fatalf("loc path: %v", err)
	} else if e.APA7.Title != "LoC Title" {
		t.Fatalf("unexpected: %+v", e)
	} else if prov != "loc" {
		t.Fatalf("prov: %s", prov)
	} else if attempts[len(attempts)-1].Provider != "loc" || !attempts[len(attempts)-1].Success {
		t.Fatalf("attempts: %+v", attempts)
	}

	// Fail LoC; expect error (no OpenAI fallback for books)
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response { return textResp(500, "err") }})
	if _, _, attempts, err := LookupBookByISBN(context.Background(), "777"); err == nil {
		t.Fatalf("expected error when all providers fail")
	} else if len(attempts) == 0 || attempts[len(attempts)-1].Success {
		t.Fatalf("expected all failures, got: %+v", attempts)
	}
}

func TestFetchCrossrefByISBN_PublishedPrintDate(t *testing.T) {
	// Directly exercise the PublishedPrint date branch
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "api.crossref.org") {
			return jsonResp(200, map[string]any{
				"message": map[string]any{
					"items": []map[string]any{{
						"title":           []string{"CR2 Title"},
						"publisher":       "CR2 Pub",
						"published-print": map[string]any{"date-parts": [][]int{{2015, 7, 9}}},
						"author":          []map[string]string{{"family": "Doe", "given": "John"}},
					}},
				},
			})
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { SetHTTPClient(&http.Client{}) })
	e, err := fetchCrossrefByISBN(context.Background(), "888")
	if err != nil {
		t.Fatalf("crossref secondary: %v", err)
	}
	if e.APA7.Year == nil || *e.APA7.Year != 2015 {
		t.Fatalf("expected year 2015, got %+v", e.APA7.Year)
	}
}

func TestLookupBookByTitleAuthor_OpenLibrarySearch(t *testing.T) {
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "openlibrary.org") && strings.Contains(req.URL.Path, "/search.json") {
			return jsonResp(200, map[string]any{"docs": []map[string]any{{
				"title": "T", "author_name": []string{"Doe, Jane"}, "publisher": []string{"Pub"}, "first_publish_year": 2001, "key": "/works/OL1W",
			}}})
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { SetHTTPClient(&http.Client{}) })
	e, prov, attempts, err := LookupBookByTitleAuthor(context.Background(), "T", "Doe, Jane")
	if err != nil {
		t.Fatalf("ol search: %v", err)
	}
	if prov != "openlibrary" || len(attempts) == 0 || !attempts[0].Success {
		t.Fatalf("unexpected prov/attempts: %s %+v", prov, attempts)
	}
	if e.APA7.Title != "T" || e.APA7.Publisher != "Pub" {
		t.Fatalf("entry: %+v", e)
	}
}

func TestLookupBookByTitleAuthor_GoogleFallback(t *testing.T) {
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "openlibrary.org") && strings.Contains(req.URL.Path, "/search.json") {
			return jsonResp(200, map[string]any{"docs": []any{}})
		}
		if strings.Contains(req.URL.Host, "googleapis.com") {
			return jsonResp(200, map[string]any{"items": []map[string]any{{"volumeInfo": map[string]any{
				"title": "TG", "authors": []string{"Doe, Jane"}, "publisher": "GP", "publishedDate": "2010", "infoLink": "https://g",
			}}}})
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { SetHTTPClient(&http.Client{}) })
	e, prov, attempts, err := LookupBookByTitleAuthor(context.Background(), "TG", "Doe")
	if err != nil {
		t.Fatalf("google: %v", err)
	}
	if prov != "googlebooks" || len(attempts) < 2 || !attempts[1].Success {
		t.Fatalf("attempts: %+v", attempts)
	}
	if e.APA7.Publisher != "GP" {
		t.Fatalf("entry: %+v", e)
	}
}

func TestLookupBookByTitleAuthor_CrossrefFallback(t *testing.T) {
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "openlibrary.org") && strings.Contains(req.URL.Path, "/search.json") {
			return jsonResp(200, map[string]any{"docs": []any{}})
		}
		if strings.Contains(req.URL.Host, "googleapis.com") {
			return jsonResp(200, map[string]any{"items": []any{}})
		}
		if strings.Contains(req.URL.Host, "api.crossref.org") {
			return jsonResp(200, map[string]any{"message": map[string]any{"items": []map[string]any{{
				"title": []string{"TC"}, "publisher": "CP", "issued": map[string]any{"date-parts": [][]int{{2018}}}, "author": []map[string]string{{"family": "Doe", "given": "J"}}, "URL": "https://cr",
			}}}})
		}
		return textResp(404, "")
	}})
	t.Cleanup(func() { SetHTTPClient(&http.Client{}) })
	e, prov, attempts, err := LookupBookByTitleAuthor(context.Background(), "TC", "Doe")
	if err != nil {
		t.Fatalf("crossref: %v", err)
	}
	if prov != "crossref" || len(attempts) < 3 || !attempts[2].Success {
		t.Fatalf("attempts: %+v", attempts)
	}
	if e.APA7.Publisher != "CP" {
		t.Fatalf("entry: %+v", e)
	}
}

func TestLookupBookByTitleAuthor_AllFail(t *testing.T) {
	SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response { return textResp(404, "") }})
	t.Cleanup(func() { SetHTTPClient(&http.Client{}) })
	if _, _, attempts, err := LookupBookByTitleAuthor(context.Background(), "X", "Y"); err == nil {
		t.Fatalf("expected error")
	} else if len(attempts) != 3 {
		t.Fatalf("expected 3 attempts, got %+v", attempts)
	}
}
