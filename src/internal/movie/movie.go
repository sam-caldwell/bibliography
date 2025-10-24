package movie

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"bibliography/src/internal/dates"
	"bibliography/src/internal/httpx"
	"bibliography/src/internal/names"
	"bibliography/src/internal/sanitize"
	"bibliography/src/internal/schema"
)

var client httpx.Doer = &http.Client{Timeout: 10 * time.Second}

// SetHTTPClient sets the HTTP client used for external movie APIs (for tests).
func SetHTTPClient(c httpx.Doer) { client = c }

// FetchMovie resolves movie metadata using OMDb first, then TMDb, returning a minimally valid Entry.
// Requires OMDB_API_KEY and/or TMDB_API_KEY to be set for those providers; otherwise they are skipped.
func FetchMovie(ctx context.Context, title string, date string) (schema.Entry, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return schema.Entry{}, fmt.Errorf("title is required")
	}
	// 1) OMDb
	if e, err := fetchFromOMDb(ctx, t, date, strings.TrimSpace(os.Getenv("OMDB_API_KEY"))); err == nil {
		return e, nil
	}
	// 2) TMDb
	if e, err := fetchFromTMDb(ctx, t, date, strings.TrimSpace(os.Getenv("TMDB_API_KEY"))); err == nil {
		return e, nil
	}
	return schema.Entry{}, fmt.Errorf("no movie metadata provider succeeded")
}

// FetchMovieWithProvider returns a movie entry and the provider label ("omdb" or "tmdb").
// It tries OMDb first, then TMDb.
func FetchMovieWithProvider(ctx context.Context, title string, date string) (schema.Entry, string, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return schema.Entry{}, "", fmt.Errorf("title is required")
	}
	if e, err := fetchFromOMDb(ctx, t, date, strings.TrimSpace(os.Getenv("OMDB_API_KEY"))); err == nil {
		return e, "omdb", nil
	}
	if e, err := fetchFromTMDb(ctx, t, date, strings.TrimSpace(os.Getenv("TMDB_API_KEY"))); err == nil {
		return e, "tmdb", nil
	}
	return schema.Entry{}, "", fmt.Errorf("no movie metadata provider succeeded")
}

// fetchFromOMDb queries OMDb by title/year and maps the response to an Entry.
func fetchFromOMDb(ctx context.Context, title string, date string, apiKey string) (schema.Entry, error) {
	if apiKey == "" {
		return schema.Entry{}, fmt.Errorf("omdb: missing api key")
	}
	req := buildOMDbRequest(ctx, title, date, apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schema.Entry{}, fmt.Errorf("omdb: http %d", resp.StatusCode)
	}
	out, err := decodeOMDb(resp)
	if err != nil {
		return schema.Entry{}, err
	}
	if strings.ToLower(out.Response) != "true" {
		if strings.TrimSpace(out.Error) != "" {
			return schema.Entry{}, fmt.Errorf("omdb: %s", out.Error)
		}
		return schema.Entry{}, fmt.Errorf("omdb: no results")
	}
	e := mapOMDbToEntry(out, title, date)
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

type omdbResp struct {
	Response   string `json:"Response"`
	Error      string `json:"Error"`
	Title      string `json:"Title"`
	Year       string `json:"Year"`
	Released   string `json:"Released"`
	Director   string `json:"Director"`
	Production string `json:"Production"`
	Plot       string `json:"Plot"`
	Website    string `json:"Website"`
	ImdbID     string `json:"imdbID"`
}

func buildOMDbRequest(ctx context.Context, title, date, apiKey string) *http.Request {
	u, _ := url.Parse("https://www.omdbapi.com/")
	q := u.Query()
	q.Set("t", title)
	q.Set("type", "movie")
	if y := dates.YearFromDate(date); y > 0 {
		q.Set("y", fmt.Sprintf("%d", y))
	}
	q.Set("apikey", apiKey)
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	httpx.SetUA(req)
	req.Header.Set("Accept", "application/json")
	return req
}

func decodeOMDb(resp *http.Response) (omdbResp, error) {
	var out omdbResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return omdbResp{}, err
	}
	return out, nil
}

func mapOMDbToEntry(out omdbResp, reqTitle, reqDate string) schema.Entry {
	var e schema.Entry
	e.Type = "movie"
	e.ID = schema.NewID()
	e.APA7.Title = strings.TrimSpace(out.Title)
	if e.APA7.Title == "" {
		e.APA7.Title = reqTitle
	}
	e.APA7.Date = parseReleasedDate(out.Released)
	if e.APA7.Date == "" {
		e.APA7.Date = strings.TrimSpace(reqDate)
	}
	if y := dates.YearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.Publisher = strings.TrimSpace(out.Production)
	for _, name := range splitComma(out.Director) {
		fam, giv := names.Split(name)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	e.Annotation.Summary = strings.TrimSpace(out.Plot)
	if e.Annotation.Summary == "" {
		e.Annotation.Summary = fmt.Sprintf("Film: %s.", e.APA7.Title)
	}
	if link := primaryLink(out); link != "" {
		e.APA7.URL = link
		e.APA7.Accessed = dates.NowISO()
	}
	e.Annotation.Keywords = []string{"movie"}
	return e
}

func parseReleasedDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse("02 Jan 2006", s); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}

func primaryLink(out omdbResp) string {
	if u := strings.TrimSpace(out.Website); u != "" {
		return u
	}
	if id := strings.TrimSpace(out.ImdbID); id != "" {
		return "https://www.imdb.com/title/" + id
	}
	return ""
}

// fetchFromTMDb queries TMDb search/credits APIs and maps the response to an Entry.
func fetchFromTMDb(ctx context.Context, title string, date string, apiKey string) (schema.Entry, error) {
	if apiKey == "" {
		return schema.Entry{}, fmt.Errorf("tmdb: missing api key")
	}
	req := buildTMDbSearchRequest(ctx, title, date, apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schema.Entry{}, fmt.Errorf("tmdb: http %d", resp.StatusCode)
	}
	out, err := decodeTMDbSearch(resp)
	if err != nil {
		return schema.Entry{}, err
	}
	if len(out.Results) == 0 {
		return schema.Entry{}, fmt.Errorf("tmdb: no results")
	}
	e := mapTMDbToEntry(ctx, out.Results[0], title, date, apiKey)
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

type tmdbSearchResp struct {
	Results []tmdbResult `json:"results"`
}
type tmdbResult struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Overview    string `json:"overview"`
	ReleaseDate string `json:"release_date"`
}

func buildTMDbSearchRequest(ctx context.Context, title, date, apiKey string) *http.Request {
	u, _ := url.Parse("https://api.themoviedb.org/3/search/movie")
	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("query", title)
	if y := dates.YearFromDate(date); y > 0 {
		q.Set("year", fmt.Sprintf("%d", y))
	}
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	httpx.SetUA(req)
	req.Header.Set("Accept", "application/json")
	return req
}

func decodeTMDbSearch(resp *http.Response) (tmdbSearchResp, error) {
	var out tmdbSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return tmdbSearchResp{}, err
	}
	return out, nil
}

func mapTMDbToEntry(ctx context.Context, r tmdbResult, title, date, apiKey string) schema.Entry {
	var e schema.Entry
	e.Type = "movie"
	e.ID = schema.NewID()
	if strings.TrimSpace(r.Title) != "" {
		e.APA7.Title = r.Title
	} else {
		e.APA7.Title = title
	}
	e.APA7.Date = strings.TrimSpace(r.ReleaseDate)
	if e.APA7.Date == "" {
		e.APA7.Date = strings.TrimSpace(date)
	}
	if y := dates.YearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.Annotation.Summary = strings.TrimSpace(r.Overview)
	if e.Annotation.Summary == "" {
		e.Annotation.Summary = fmt.Sprintf("Film: %s.", e.APA7.Title)
	}
	if r.ID != 0 {
		for _, name := range fetchTMDbDirectors(ctx, r.ID, apiKey) {
			fam, giv := names.Split(name)
			if fam != "" {
				e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
			}
		}
		e.APA7.URL = fmt.Sprintf("https://www.themoviedb.org/movie/%d", r.ID)
		e.APA7.Accessed = dates.NowISO()
	}
	e.Annotation.Keywords = []string{"movie"}
	return e
}

func fetchTMDbDirectors(ctx context.Context, movieID int, apiKey string) []string {
	cu := fmt.Sprintf("https://api.themoviedb.org/3/movie/%d/credits?api_key=%s", movieID, apiKey)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cu, nil)
	httpx.SetUA(req)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp != nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()
	var c struct {
		Crew []struct{ Job, Name string } `json:"crew"`
	}
	if json.NewDecoder(resp.Body).Decode(&c) != nil {
		return nil
	}
	var namesOut []string
	for _, m := range c.Crew {
		if strings.EqualFold(strings.TrimSpace(m.Job), "Director") {
			namesOut = append(namesOut, m.Name)
		}
	}
	return namesOut
}

// splitComma splits a comma-delimited string and trims non-empty parts.
func splitComma(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitName/initials removed; use names.Split / names.Initials
