package movie

import (
    "context"
    "encoding/json"
    "io"
    "net/http"
    "strings"
    "testing"
)

type fakeDoer struct{}

func (fakeDoer) Do(req *http.Request) (*http.Response, error) {
    if strings.Contains(req.URL.Host, "www.omdbapi.com") {
        // Success path
        out := map[string]any{
            "Response": "True",
            "Title": "Film",
            "Released": "02 Jan 2020",
            "Director": "Jane Doe",
            "Production": "Studio",
            "Plot": "Summary",
            "Website": "https://example.com/film",
            "imdbID": "tt123",
        }
        b, _ := json.Marshal(out)
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
    }
    return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
}

func TestFetchMovie_FromOMDb(t *testing.T) {
    SetHTTPClient(fakeDoer{})
    t.Setenv("OMDB_API_KEY", "x")
    e, err := FetchMovie(context.Background(), "Film", "2020-01-02")
    if err != nil { t.Fatalf("FetchMovie: %v", err) }
    if e.Type != "movie" || e.APA7.Title == "" { t.Fatalf("invalid entry: %+v", e) }
    if e.APA7.URL == "" { t.Fatalf("expected url populated") }
}

type fakeDoerFallback struct{}

func (fakeDoerFallback) Do(req *http.Request) (*http.Response, error) {
    if strings.Contains(req.URL.Host, "www.omdbapi.com") {
        return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("err")), Header: make(http.Header)}, nil
    }
    if strings.Contains(req.URL.Host, "api.themoviedb.org") && strings.Contains(req.URL.Path, "/search/movie") {
        body := `{"results":[{"id":123,"title":"Film","overview":"O","release_date":"2020-01-02"}]}`
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
    }
    if strings.Contains(req.URL.Host, "api.themoviedb.org") && strings.Contains(req.URL.Path, "/credits") {
        body := `{"crew":[{"job":"Director","name":"Jane Doe"}]}`
        return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
    }
    return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
}

func TestFetchMovie_TMDbFallback(t *testing.T) {
    SetHTTPClient(fakeDoerFallback{})
    t.Setenv("TMDB_API_KEY", "x")
    e, err := FetchMovie(context.Background(), "Film", "2020-01-02")
    if err != nil { t.Fatalf("TMDb fallback: %v", err) }
    if e.APA7.Title == "" || len(e.APA7.Authors) == 0 { t.Fatalf("expected director from credits: %+v", e) }
}
