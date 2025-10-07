package song

import (
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/url"
    "strings"
    "testing"
)

type fakeDoer struct{}

func (fakeDoer) Do(req *http.Request) (*http.Response, error) {
    // Return iTunes result for itunes host, else MusicBrainz
    if strings.Contains(req.URL.Host, "itunes.apple.com") {
        r := struct{ ResultCount int; Results []map[string]any }{1, []map[string]any{{
            "trackName": "Track",
            "artistName": "Artist",
            "collectionName": "Album",
            "trackViewUrl": "https://itunes.apple.com/track",
            "releaseDate": "2021-01-02T00:00:00Z",
        }}}
        b, _ := json.Marshal(r)
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
    }
    if strings.Contains(req.URL.Host, "musicbrainz.org") {
        // not used in this test, but supply minimal payload
        payload := `{"recordings":[{"title":"T","artist-credit":[{"name":"A"}],"releases":[{"title":"R","date":"2020-01-01","label-info":[{"label":{"name":"L"}}]}]}]}`
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
    }
    return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
}

func TestFetchSong_FromITunes(t *testing.T) {
    SetHTTPClient(fakeDoer{})
    e, err := FetchSong(context.Background(), "Title", "Artist", "2021-01-02")
    if err != nil { t.Fatalf("FetchSong: %v", err) }
    if e.Type != "song" || e.APA7.Title == "" || e.APA7.URL == "" { t.Fatalf("entry not populated: %+v", e) }
    if _, err := url.Parse(e.APA7.URL); err != nil { t.Fatalf("invalid url: %v", err) }
}

type fakeDoerMB struct{}

func (fakeDoerMB) Do(req *http.Request) (*http.Response, error) {
    if strings.Contains(req.URL.Host, "itunes.apple.com") {
        // no results to force fallback
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"resultCount":0,"results":[]}`)), Header: make(http.Header)}, nil
    }
    if strings.Contains(req.URL.Host, "musicbrainz.org") {
        payload := `{"recordings":[{"title":"T","artist-credit":[{"name":"A"}],"releases":[{"title":"R","date":"2020-01-01","label-info":[{"label":{"name":"L"}}]}]}]}`
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header)}, nil
    }
    return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
}

func TestFetchSong_MusicBrainzFallback(t *testing.T) {
    SetHTTPClient(fakeDoerMB{})
    e, err := FetchSong(context.Background(), "Title", "Artist", "")
    if err != nil { t.Fatalf("MusicBrainz fallback: %v", err) }
    if e.APA7.Title == "" || e.APA7.ContainerTitle == "" { t.Fatalf("expected fields from MB: %+v", e) }
}
