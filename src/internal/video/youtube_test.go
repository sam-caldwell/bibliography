package video

import (
    "context"
    "io"
    "net/http"
    "strings"
    "testing"
)

type fakeDoer struct{ status int; body string }
func (f fakeDoer) Do(req *http.Request) (*http.Response, error) {
    if f.status == 0 { f.status = 200 }
    if f.body == "" {
        f.body = `{"title":"Cool","author_name":"Chan","provider_name":"YouTube"}`
    }
    return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(f.body)), Header: make(http.Header)}, nil
}

func TestFetchYouTube_OEmbed(t *testing.T) {
    SetHTTPClient(fakeDoer{})
    e, err := FetchYouTube(context.Background(), "https://www.youtube.com/watch?v=abc")
    if err != nil { t.Fatalf("FetchYouTube: %v", err) }
    if e.Type != "video" || e.APA7.Title == "" || e.APA7.URL == "" { t.Fatalf("entry not populated: %+v", e) }
}
