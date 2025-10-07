package video

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bibliography/src/internal/dates"
	"bibliography/src/internal/httpx"
	"bibliography/src/internal/sanitize"
	"bibliography/src/internal/schema"
)

var client httpx.Doer = &http.Client{Timeout: 10 * time.Second}

// SetHTTPClient sets the HTTP client used for YouTube oEmbed requests (for tests).
func SetHTTPClient(c httpx.Doer) { client = c }

// chrome UA constant removed; use httpx.SetUA

// FetchYouTube fetches minimal metadata for a YouTube video via the oEmbed endpoint.
// It constructs a valid video entry with at least title, channel (as corporate author),
// YouTube as container/publisher, URL and accessed, and a basic summary.
func FetchYouTube(ctx context.Context, pageURL string) (schema.Entry, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return schema.Entry{}, fmt.Errorf("invalid youtube url")
	}
	// Use oEmbed JSON endpoint
	ou, _ := url.Parse("https://www.youtube.com/oembed")
	q := ou.Query()
	q.Set("format", "json")
	q.Set("url", pageURL)
	ou.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ou.String(), nil)
	httpx.SetUA(req)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return schema.Entry{}, fmt.Errorf("youtube oembed: http %d", resp.StatusCode)
	}
	var out struct {
		Title      string `json:"title"`
		AuthorName string `json:"author_name"`
		Provider   string `json:"provider_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	var e schema.Entry
	e.Type = "video"
	e.ID = schema.NewID()
	e.APA7.Title = strings.TrimSpace(out.Title)
	if e.APA7.Title == "" {
		e.APA7.Title = pageURL
	}
	// Channel as corporate author
	if a := strings.TrimSpace(out.AuthorName); a != "" {
		e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: a})
	}
	// Set container/publisher
	e.APA7.ContainerTitle = "YouTube"
	e.APA7.Publisher = "YouTube"
	// URL + accessed
	e.APA7.URL = pageURL
	e.APA7.Accessed = dates.NowISO()
	// Minimal summary and keyword
	if a := strings.TrimSpace(out.AuthorName); a != "" {
		e.Annotation.Summary = fmt.Sprintf("YouTube video: %s by %s.", e.APA7.Title, a)
	} else {
		e.Annotation.Summary = fmt.Sprintf("YouTube video: %s.", e.APA7.Title)
	}
	e.Annotation.Keywords = []string{"video"}
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}
