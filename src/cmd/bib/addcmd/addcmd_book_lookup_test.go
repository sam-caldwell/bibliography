package addcmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"bibliography/src/internal/booksearch"
)

type fakeDoer2 struct {
	handler func(req *http.Request) *http.Response
}

func (f fakeDoer2) Do(req *http.Request) (*http.Response, error) { return f.handler(req), nil }

func jsonResp2(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: ioNopCloser{Reader: strings.NewReader(body)}, Header: http.Header{"Content-Type": {"application/json"}}}
}

type ioNopCloser struct{ Reader *strings.Reader }

func (n ioNopCloser) Read(p []byte) (int, error) { return n.Reader.Read(p) }
func (n ioNopCloser) Close() error               { return nil }

func TestAddBook_TitleAuthorLookup(t *testing.T) {
	// Stub OpenLibrary search
	booksearch.SetHTTPClient(fakeDoer2{handler: func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Host, "openlibrary.org") && strings.Contains(req.URL.Path, "/search.json") {
			return jsonResp2(200, `{"docs":[{"title":"TT","author_name":["Doe, Jane"],"publisher":["P"],"first_publish_year":2002,"key":"/works/OL1W"}]}`)
		}
		return &http.Response{StatusCode: 404, Body: ioNopCloser{strings.NewReader("")}}
	}})
	t.Cleanup(func() { booksearch.SetHTTPClient(&http.Client{}) })

	b := New(func(paths []string, msg string) error { return nil })
	cmd := b.Book()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--name", "TT", "--author", "Doe, Jane", "--lookup"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lookup add book: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "tried: openlibrary-search: status: found") || !strings.Contains(out, "source: openlibrary") || !strings.Contains(out, "wrote ") {
		t.Fatalf("unexpected stdout: %s", out)
	}
}
