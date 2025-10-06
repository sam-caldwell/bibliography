package song

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// FetchSong tries iTunes Search API first, then MusicBrainz. Returns a minimally valid APA7 entry of type "song".
func FetchSong(ctx context.Context, title string, artist string, date string) (schema.Entry, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return schema.Entry{}, fmt.Errorf("title is required")
	}
	if e, err := fetchFromITunes(ctx, t, artist, date); err == nil {
		return e, nil
	}
	if e, err := fetchFromMusicBrainz(ctx, t, artist, date); err == nil {
		return e, nil
	}
	return schema.Entry{}, fmt.Errorf("no song metadata provider succeeded")
}

func fetchFromITunes(ctx context.Context, title, artist, date string) (schema.Entry, error) {
	term := strings.TrimSpace(title + " " + artist)
	u, _ := url.Parse("https://itunes.apple.com/search")
	q := u.Query()
	q.Set("term", term)
	q.Set("entity", "song")
	q.Set("limit", "1")
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
		return schema.Entry{}, fmt.Errorf("itunes: http %d", resp.StatusCode)
	}
	var out struct {
		ResultCount int `json:"resultCount"`
		Results     []struct {
			TrackName      string `json:"trackName"`
			ArtistName     string `json:"artistName"`
			CollectionName string `json:"collectionName"`
			TrackViewURL   string `json:"trackViewUrl"`
			ReleaseDate    string `json:"releaseDate"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if out.ResultCount == 0 || len(out.Results) == 0 {
		return schema.Entry{}, fmt.Errorf("itunes: no results")
	}
	r := out.Results[0]
	var e schema.Entry
	e.Type = "song"
	e.ID = schema.NewID()
	e.APA7.Title = firstNonEmpty(r.TrackName, title)
	// Authors -> performer
	if a := strings.TrimSpace(r.ArtistName); a != "" {
		e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: a})
	}
	e.APA7.ContainerTitle = strings.TrimSpace(r.CollectionName) // album
	// Date
	d := strings.TrimSpace(r.ReleaseDate)
	if len(d) >= 10 {
		e.APA7.Date = d[:10]
	}
	if e.APA7.Date == "" {
		e.APA7.Date = strings.TrimSpace(date)
	}
	if y := yearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	// URL
	if u := strings.TrimSpace(r.TrackViewURL); u != "" {
		e.APA7.URL = u
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	}
	// Summary / keywords
	e.Annotation.Summary = "Song: " + e.APA7.Title + "."
	e.Annotation.Keywords = []string{"song"}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func fetchFromMusicBrainz(ctx context.Context, title, artist, date string) (schema.Entry, error) {
	// Build query
	q := "recording:" + quote(title)
	if strings.TrimSpace(artist) != "" {
		q += " AND artist:" + quote(artist)
	}
	u, _ := url.Parse("https://musicbrainz.org/ws/2/recording/")
	qq := u.Query()
	qq.Set("query", q)
	qq.Set("fmt", "json")
	qq.Set("limit", "1")
	u.RawQuery = qq.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schema.Entry{}, fmt.Errorf("musicbrainz: http %d", resp.StatusCode)
	}
	var out struct {
		Recordings []struct {
			Title        string `json:"title"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
			Releases []struct {
				Title     string `json:"title"`
				Date      string `json:"date"`
				LabelInfo []struct {
					Label struct {
						Name string `json:"name"`
					} `json:"label"`
				} `json:"label-info"`
			} `json:"releases"`
		} `json:"recordings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Recordings) == 0 {
		return schema.Entry{}, fmt.Errorf("musicbrainz: no results")
	}
	r := out.Recordings[0]
	var e schema.Entry
	e.Type = "song"
	e.ID = schema.NewID()
	e.APA7.Title = firstNonEmpty(r.Title, title)
	if len(r.ArtistCredit) > 0 {
		a := strings.TrimSpace(r.ArtistCredit[0].Name)
		if a != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: a})
		}
	}
	if len(r.Releases) > 0 {
		rel := r.Releases[0]
		if strings.TrimSpace(rel.Title) != "" {
			e.APA7.ContainerTitle = strings.TrimSpace(rel.Title)
		}
		if strings.TrimSpace(rel.Date) != "" {
			e.APA7.Date = strings.TrimSpace(rel.Date)
		}
		if len(rel.LabelInfo) > 0 && strings.TrimSpace(rel.LabelInfo[0].Label.Name) != "" {
			e.APA7.Publisher = strings.TrimSpace(rel.LabelInfo[0].Label.Name)
		}
	}
	if e.APA7.Date == "" {
		e.APA7.Date = strings.TrimSpace(date)
	}
	if y := yearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.Annotation.Summary = "Song: " + e.APA7.Title + "."
	e.Annotation.Keywords = []string{"song"}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func quote(s string) string {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " ") {
		return "\"" + s + "\""
	}
	return s
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
