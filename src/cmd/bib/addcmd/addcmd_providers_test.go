package addcmd

import (
	"bibliography/src/internal/store"
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"bibliography/src/internal/doi"
	moviefetch "bibliography/src/internal/movie"
	"bibliography/src/internal/openlibrary"
	rfcpkg "bibliography/src/internal/rfc"
	"bibliography/src/internal/schema"
	songfetch "bibliography/src/internal/song"
	youtube "bibliography/src/internal/video"
	"bibliography/src/internal/webfetch"
)

// fakeDoer implements httpx.Doer for deterministic responses in tests.
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

func TestAdd_Providers_DOI_YouTube_OMDb_TMDb_RFC_ISBN(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Commit stub
	commits := 0
	b := New(func(paths []string, msg string) error { commits++; return nil })

	// 1) DOI article via doi.org
	doi.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "doi.org") {
			// Minimal CSL JSON
			payload := map[string]any{
				"title":           "Test Article",
				"container-title": "Journal X",
				"issued":          map[string]any{"date-parts": [][]int{{2021, 2, 3}}},
				"author":          []map[string]string{{"family": "Doe", "given": "Jane"}},
				"DOI":             "10.1234/abc",
			}
			return jsonResp(200, payload)
		}
		return textResp(404, "not found")
	}})
	art := b.Article()
	art.SetArgs([]string{"--doi", "10.1234/abc"})
	art.SetOut(new(bytes.Buffer))
	if err := art.Execute(); err != nil {
		t.Fatalf("article doi: %v", err)
	}

	// Article via URL using webfetch
	webfetch.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "news.example.com") {
			html := `<!doctype html><html><head>
            <meta property="og:title" content="News Title">
            <meta property="og:site_name" content="Example News">
            <meta name="author" content="Jane Doe">
            <meta name="description" content="Desc">
            </head><body></body></html>`
			return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader(html)), Header: http.Header{"Content-Type": {"text/html"}}}
		}
		return textResp(404, "")
	}})
	art2 := b.Article()
	art2.SetArgs([]string{"--url", "https://news.example.com/post"})
	art2.SetOut(new(bytes.Buffer))
	if err := art2.Execute(); err != nil {
		t.Fatalf("article url: %v", err)
	}

	// 2) YouTube video via oEmbed
	youtube.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "youtube.com") && strings.Contains(req.URL.Path, "/oembed") {
			return jsonResp(200, map[string]any{"title": "YTitle", "author_name": "Channel", "provider_name": "YouTube"})
		}
		return textResp(404, "")
	}})
	vid := b.Video()
	vid.SetArgs([]string{"--youtube", "https://www.youtube.com/watch?v=xyz"})
	vid.SetOut(new(bytes.Buffer))
	if err := vid.Execute(); err != nil {
		t.Fatalf("video youtube: %v", err)
	}

	// 3) Movie via OMDb
	t.Setenv("OMDB_API_KEY", "k")
	moviefetch.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "omdbapi.com") {
			return jsonResp(200, map[string]any{
				"Response":   "True",
				"Title":      "OMDb Film",
				"Released":   "2019-02-03",
				"Director":   "Jane Doe",
				"Production": "ACME",
				"Plot":       "Plot.",
				"Website":    "https://film.example/",
				"imdbID":     "tt123",
			})
		}
		return textResp(404, "")
	}})
	mov := b.Movie()
	mov.SetArgs([]string{"OMDb Film"})
	mov.SetOut(new(bytes.Buffer))
	if err := mov.Execute(); err != nil {
		t.Fatalf("movie omdb: %v", err)
	}

	// 4) Movie via TMDb fallback when OMDb fails
	t.Setenv("TMDB_API_KEY", "k2")
	moviefetch.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "omdbapi.com") { // force failure
			return textResp(500, "err")
		}
		if strings.Contains(req.URL.Host, "api.themoviedb.org") && strings.Contains(req.URL.Path, "/search/movie") {
			return jsonResp(200, map[string]any{"results": []map[string]any{{
				"id": 42, "title": "TMDb Film", "overview": "Desc", "release_date": "2018-04-05",
			}}})
		}
		if strings.Contains(req.URL.Host, "api.themoviedb.org") && strings.Contains(req.URL.Path, "/credits") {
			return jsonResp(200, map[string]any{"crew": []map[string]any{{"job": "Director", "name": "John Roe"}}})
		}
		return textResp(404, "")
	}})
	mov2 := b.Movie()
	mov2.SetArgs([]string{"TMDb Film"})
	mov2.SetOut(new(bytes.Buffer))
	if err := mov2.Execute(); err != nil {
		t.Fatalf("movie tmdb: %v", err)
	}

	// 5) RFC by number (use BibTeX path)
	rfcpkg.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		// Force XML path (return 404 for bibtex)
		if strings.Contains(req.URL.Host, "datatracker.ietf.org") && strings.Contains(req.URL.Path, "/bibtex/") {
			return textResp(404, "not")
		}
		if strings.Contains(req.URL.Host, "www.rfc-editor.org") && strings.Contains(req.URL.Path, "/rfc/rfc9999.xml") {
			xml := `<?xml version="1.0"?>
<rfc>
  <front>
    <title>Test RFC</title>
    <author fullname="Foo Bar"><name><surname>Bar</surname><given>Foo</given></name></author>
    <date year="2020" month="January" day="15"/>
    <seriesInfo name="RFC" value="9999"/>
  </front>
</rfc>`
			return &http.Response{StatusCode: 200, Body: ioutil.NopCloser(strings.NewReader(xml)), Header: http.Header{"Content-Type": {"application/xml"}}}
		}
		// Credits/json not used in RFC
		return textResp(404, "")
	}})
	rfc := b.RFC()
	rfc.SetArgs([]string{"9999"})
	rfc.SetOut(new(bytes.Buffer))
	if err := rfc.Execute(); err != nil {
		t.Fatalf("rfc: %v", err)
	}

	// 6) Book by ISBN via OpenLibrary (jscmd=data)
	openlibrary.SetHTTPClient(fakeDoer{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "openlibrary.org") && strings.Contains(req.URL.RawQuery, "jscmd=data") {
			key := "ISBN:1234567890"
			payload := map[string]any{
				key: map[string]any{
					"title":        "OL Book",
					"publishers":   []map[string]string{{"name": "Pub"}},
					"publish_date": "2001",
					"authors":      []map[string]string{{"name": "Doe, Jane"}},
					"url":          "https://openlibrary.org/works/OL1W",
				},
			}
			return jsonResp(200, payload)
		}
		return textResp(404, "")
	}})
	book := b.Book()
	book.SetArgs([]string{"--isbn", "1234567890"})
	book.SetOut(new(bytes.Buffer))
	if err := book.Execute(); err != nil {
		t.Fatalf("book isbn: %v", err)
	}

	// Ensure bib file was written
	if _, err := os.Stat(store.BibFile); err != nil {
		t.Fatalf("missing bib file: %v", err)
	}
	if commits < 6 {
		t.Fatalf("expected commits, got %d", commits)
	}

	// Silence unused imports
	_ = context.Background()
	_ = schema.Entry{}
	_ = songfetch.SetHTTPClient
}
