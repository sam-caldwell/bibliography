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

	"bibliography/src/internal/schema"
)

// HTTPDoer allows injection for tests.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var client HTTPDoer = &http.Client{Timeout: 10 * time.Second}

func SetHTTPClient(c HTTPDoer) { client = c }

const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

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

func fetchFromOMDb(ctx context.Context, title string, date string, apiKey string) (schema.Entry, error) {
	if apiKey == "" {
		return schema.Entry{}, fmt.Errorf("omdb: missing api key")
	}
	u, _ := url.Parse("https://www.omdbapi.com/")
	q := u.Query()
	q.Set("t", title)
	q.Set("type", "movie")
	if y := yearFromDate(date); y > 0 {
		q.Set("y", fmt.Sprintf("%d", y))
	}
	q.Set("apikey", apiKey)
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schema.Entry{}, fmt.Errorf("omdb: http %d", resp.StatusCode)
	}
	var out struct {
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
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if strings.ToLower(out.Response) != "true" {
		if out.Error != "" {
			return schema.Entry{}, fmt.Errorf("omdb: %s", out.Error)
		}
		return schema.Entry{}, fmt.Errorf("omdb: no results")
	}
	var e schema.Entry
	e.Type = "movie"
	e.ID = schema.NewID()
	if strings.TrimSpace(out.Title) != "" {
		e.APA7.Title = out.Title
	} else {
		e.APA7.Title = title
	}
	// Released -> date (e.g., 27 Apr 1957)
	if d := strings.TrimSpace(out.Released); d != "" {
		if t, err := time.Parse("02 Jan 2006", d); err == nil {
			e.APA7.Date = t.Format("2006-01-02")
		}
	}
	if e.APA7.Date == "" {
		e.APA7.Date = strings.TrimSpace(date)
	}
	if y := yearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.Publisher = strings.TrimSpace(out.Production)
	// Directors -> authors (comma-separated)
	for _, name := range splitComma(out.Director) {
		fam, giv := splitName(name)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	e.Annotation.Summary = strings.TrimSpace(out.Plot)
	if e.Annotation.Summary == "" {
		e.Annotation.Summary = fmt.Sprintf("Film: %s.", e.APA7.Title)
	}
	// Prefer Website; else link to IMDb page
	link := strings.TrimSpace(out.Website)
	if link == "" && strings.TrimSpace(out.ImdbID) != "" {
		link = "https://www.imdb.com/title/" + strings.TrimSpace(out.ImdbID)
	}
	if link != "" {
		e.APA7.URL = link
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	}
	e.Annotation.Keywords = []string{"movie"}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func fetchFromTMDb(ctx context.Context, title string, date string, apiKey string) (schema.Entry, error) {
	if apiKey == "" {
		return schema.Entry{}, fmt.Errorf("tmdb: missing api key")
	}
	u, _ := url.Parse("https://api.themoviedb.org/3/search/movie")
	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("query", title)
	if y := yearFromDate(date); y > 0 {
		q.Set("year", fmt.Sprintf("%d", y))
	}
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schema.Entry{}, fmt.Errorf("tmdb: http %d", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			ID          int    `json:"id"`
			Title       string `json:"title"`
			Overview    string `json:"overview"`
			ReleaseDate string `json:"release_date"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Results) == 0 {
		return schema.Entry{}, fmt.Errorf("tmdb: no results")
	}
	r := out.Results[0]
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
	if y := yearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.Annotation.Summary = strings.TrimSpace(r.Overview)
	if e.Annotation.Summary == "" {
		e.Annotation.Summary = fmt.Sprintf("Film: %s.", e.APA7.Title)
	}
	// Attempt to fetch director names via credits
	if r.ID != 0 {
		cu := fmt.Sprintf("https://api.themoviedb.org/3/movie/%d/credits?api_key=%s", r.ID, apiKey)
		creq, _ := http.NewRequestWithContext(ctx, http.MethodGet, cu, nil)
		creq.Header.Set("User-Agent", chromeUA)
		creq.Header.Set("Accept", "application/json")
		if cresp, err := client.Do(creq); err == nil && cresp.StatusCode >= 200 && cresp.StatusCode < 300 {
			defer cresp.Body.Close()
			var c struct {
				Crew []struct{ Job, Name string } `json:"crew"`
			}
			if json.NewDecoder(cresp.Body).Decode(&c) == nil {
				for _, m := range c.Crew {
					if strings.EqualFold(strings.TrimSpace(m.Job), "Director") {
						fam, giv := splitName(m.Name)
						if fam != "" {
							e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
						}
					}
				}
			}
		}
	}
	// Provide a stable link to TMDb
	if r.ID != 0 {
		e.APA7.URL = fmt.Sprintf("https://www.themoviedb.org/movie/%d", r.ID)
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	}
	e.Annotation.Keywords = []string{"movie"}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func yearFromDate(date string) int {
	date = strings.TrimSpace(date)
	if len(date) >= 4 {
		var y int
		if _, err := fmt.Sscanf(date[:4], "%d", &y); err == nil {
			return y
		}
	}
	return 0
}

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

func splitName(name string) (family, given string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	if i := strings.Index(name, ","); i >= 0 {
		family = strings.TrimSpace(name[:i])
		given = strings.TrimSpace(name[i+1:])
		return family, initials(given)
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		return parts[0], ""
	}
	family = parts[len(parts)-1]
	given = strings.Join(parts[:len(parts)-1], " ")
	return family, initials(given)
}

func initials(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var out []string
	for _, w := range strings.Fields(s) {
		r := []rune(w)
		if len(r) == 0 {
			continue
		}
		out = append(out, strings.ToUpper(string(r[0]))+".")
	}
	return strings.Join(out, " ")
}
