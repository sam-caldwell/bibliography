package doi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bibliography/src/internal/schema"
)

// HTTPDoer abstracts http.Client for testability
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

var client HTTPDoer = &http.Client{Timeout: 10 * time.Second}

const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// SetHTTPClient allows tests to inject a fake HTTP client.
func SetHTTPClient(c HTTPDoer) { client = c }

// FetchArticleByDOI uses doi.org content negotiation (CSL JSON) to build an Entry.
func FetchArticleByDOI(ctx context.Context, doi string) (schema.Entry, error) {
	u := "https://doi.org/" + strings.TrimSpace(doi)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "application/vnd.citationstyles.csl+json")
	req.Header.Set("User-Agent", chromeUA)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("doi: http %d: %s", resp.StatusCode, string(b))
	}
	var csl CSL
	if err := json.NewDecoder(resp.Body).Decode(&csl); err != nil {
		return schema.Entry{}, err
	}
	e := mapCSLToEntry(csl)
	// Canonical URL: use doi.org link per requirement
	e.APA7.URL = u
	e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	if strings.TrimSpace(e.ID) == "" {
		e.ID = schema.NewID()
	}
	// Ensure at least one keyword; default to ["article"]
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{"article"}
	}
	if strings.TrimSpace(e.Annotation.Summary) == "" {
		j := e.APA7.Journal
		if j == "" {
			j = e.APA7.ContainerTitle
		}
		if j != "" {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s in %s via DOI metadata.", e.APA7.Title, j)
		} else {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s via DOI metadata.", e.APA7.Title)
		}
	}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// CSL is a partial model of the citationstyles JSON
type CSL struct {
	Title          any         `json:"title"`
	Author         []CSLAuthor `json:"author"`
	ContainerTitle any         `json:"container-title"`
	Issued         CSLIssued   `json:"issued"`
	Volume         string      `json:"volume"`
	Issue          string      `json:"issue"`
	Page           string      `json:"page"`
	DOI            string      `json:"DOI"`
	URL            string      `json:"URL"`
	Publisher      string      `json:"publisher"`
	Type           string      `json:"type"`
}

type CSLAuthor struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

type CSLIssued struct {
	DateParts [][]int `json:"date-parts"`
}

func mapCSLToEntry(c CSL) schema.Entry {
	var e schema.Entry
	e.Type = "article"
	e.APA7.Title = toString(c.Title)
	e.APA7.ContainerTitle = toString(c.ContainerTitle)
	e.APA7.Journal = e.APA7.ContainerTitle
	if y, md := yearAndDate(c.Issued); y > 0 {
		e.APA7.Year = &y
		if md != "" {
			e.APA7.Date = md
		}
	}
	e.APA7.Volume = c.Volume
	e.APA7.Issue = c.Issue
	e.APA7.Pages = c.Page
	e.APA7.DOI = strings.TrimSpace(c.DOI)
	e.APA7.Publisher = c.Publisher
	for _, a := range c.Author {
		if strings.TrimSpace(a.Family) == "" {
			continue
		}
		e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: a.Family, Given: toInitials(a.Given)})
	}
	return e
}

func toInitials(given string) string {
	given = strings.TrimSpace(given)
	if given == "" {
		return ""
	}
	parts := strings.Fields(given)
	var out []string
	for _, p := range parts {
		r := []rune(p)
		if len(r) == 0 {
			continue
		}
		out = append(out, strings.ToUpper(string(r[0]))+".")
	}
	return strings.Join(out, " ")
}

func yearAndDate(i CSLIssued) (int, string) {
	if len(i.DateParts) == 0 || len(i.DateParts[0]) == 0 {
		return 0, ""
	}
	dp := i.DateParts[0]
	y := dp[0]
	// format YYYY-MM-DD if available
	if len(dp) >= 3 {
		return y, fmt.Sprintf("%04d-%02d-%02d", dp[0], dp[1], dp[2])
	}
	if len(dp) == 2 {
		return y, fmt.Sprintf("%04d-%02d-01", dp[0], dp[1])
	}
	return y, ""
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}
