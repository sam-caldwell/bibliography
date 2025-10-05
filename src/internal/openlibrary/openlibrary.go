package openlibrary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bibliography/src/internal/schema"
)

// HTTPDoer abstracts http.Client for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var client HTTPDoer = &http.Client{Timeout: 10 * time.Second}

// SetHTTPClient allows tests to inject a fake HTTP client.
func SetHTTPClient(c HTTPDoer) { client = c }

// FetchBookByISBN queries OpenLibrary and maps the response to schema.Entry.
func FetchBookByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
	// Use Books API with jscmd=data for simpler shape
	q := url.Values{}
	q.Set("bibkeys", "ISBN:"+strings.ReplaceAll(isbn, " ", ""))
	q.Set("format", "json")
	q.Set("jscmd", "data")
	endpoint := "https://openlibrary.org/api/books?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("openlibrary: http %d: %s", resp.StatusCode, string(b))
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return schema.Entry{}, err
	}
	key := "ISBN:" + strings.ReplaceAll(isbn, " ", "")
	dataRaw, ok := raw[key]
	if !ok || len(dataRaw) == 0 {
		return schema.Entry{}, fmt.Errorf("openlibrary: no data for %s", key)
	}
	var data struct {
		Title       string `json:"title"`
		PublishDate string `json:"publish_date"`
		URL         string `json:"url"`
		Authors     []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Publishers []struct {
			Name string `json:"name"`
		} `json:"publishers"`
		Subjects []struct {
			Name string `json:"name"`
		} `json:"subjects"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return schema.Entry{}, err
	}

	// Map to schema.Entry
	var e schema.Entry
	e.Type = "book"
	e.APA7.Title = data.Title
	if len(data.Publishers) > 0 {
		e.APA7.Publisher = data.Publishers[0].Name
	}
	e.APA7.ISBN = strings.ReplaceAll(isbn, " ", "")
	if strings.TrimSpace(data.URL) != "" {
		e.APA7.URL = data.URL
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	}
	// Parse year from publish_date if possible (may be "2008" or "May 2008")
	if y := extractYear(data.PublishDate); y > 0 {
		e.APA7.Year = &y
	}
	// Authors: set family = last token, given = initials of others (optional)
	for _, a := range data.Authors {
		fam, giv := splitName(a.Name)
		e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
	}
	// Keywords from subjects; fallback to ["book"] if none
	for _, s := range data.Subjects {
		name := strings.TrimSpace(s.Name)
		if name != "" {
			e.Annotation.Keywords = append(e.Annotation.Keywords, strings.ToLower(name))
		}
	}
	// Enrich from details subjects if available (no OpenAI calls)
	desc, moreKs := fetchDescriptionFallback(ctx, isbn)
	if len(moreKs) > 0 {
		e.Annotation.Keywords = append(e.Annotation.Keywords, moreKs...)
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{"book"}
	}
	// Try to enrich with description via details/works; fallback to neutral summary
	if strings.TrimSpace(desc) != "" {
		e.Annotation.Summary = desc
	} else {
		if e.APA7.Title != "" {
			yearStr := ""
			if e.APA7.Year != nil {
				yearStr = fmt.Sprintf("%d", *e.APA7.Year)
			}
			if yearStr != "" && e.APA7.Publisher != "" {
				e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s, %s) from OpenLibrary.", e.APA7.Title, e.APA7.Publisher, yearStr)
			} else if e.APA7.Publisher != "" {
				e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s) from OpenLibrary.", e.APA7.Title, e.APA7.Publisher)
			} else {
				e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from OpenLibrary.", e.APA7.Title)
			}
		} else {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record from OpenLibrary for ISBN %s.", isbn)
		}
	}
	// Compute ID if missing
	if strings.TrimSpace(e.ID) == "" {
		e.ID = schema.Slugify(e.APA7.Title, e.APA7.Year)
	}
	// Validate before returning
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// fetchDescriptionFallback attempts to retrieve a richer description by calling
// the OpenLibrary Books API with jscmd=details, and if a work key is present,
// fetches the work JSON to extract description. Errors are swallowed; returns
// empty string if no description can be obtained.
func fetchDescriptionFallback(ctx context.Context, isbn string) (string, []string) {
	// details endpoint
	q := url.Values{}
	q.Set("bibkeys", "ISBN:"+strings.ReplaceAll(isbn, " ", ""))
	q.Set("format", "json")
	q.Set("jscmd", "details")
	endpoint := "https://openlibrary.org/api/books?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", nil
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", nil
	}
	key := "ISBN:" + strings.ReplaceAll(isbn, " ", "")
	entryRaw, ok := raw[key]
	if !ok || len(entryRaw) == 0 {
		return "", nil
	}
	var entry struct {
		Details struct {
			Description any `json:"description"`
			Works       []struct {
				Key string `json:"key"`
			} `json:"works"`
			Subjects any `json:"subjects"`
		} `json:"details"`
	}
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		return "", nil
	}
	// Prefer details.description if present
	var subjects []string
	if sub := parseSubjects(entry.Details.Subjects); len(sub) > 0 {
		subjects = sub
	}
	if d := toDescription(entry.Details.Description); d != "" {
		return d, subjects
	}
	// Fallback to work description
	if len(entry.Details.Works) > 0 {
		if w := fetchWorkDescription(ctx, entry.Details.Works[0].Key); w != "" {
			return w, subjects
		}
	}
	return "", subjects
}

func fetchWorkDescription(ctx context.Context, workKey string) string {
	workKey = strings.TrimSpace(workKey)
	if workKey == "" {
		return ""
	}
	if !strings.HasPrefix(workKey, "/") {
		workKey = "/" + workKey
	}
	endpoint := "https://openlibrary.org" + workKey + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return ""
	}
	if d, ok := m["description"]; ok {
		return toDescription(d)
	}
	return ""
}

func toDescription(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		if s, ok := t["value"].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func parseSubjects(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, it := range t {
			switch s := it.(type) {
			case string:
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, strings.ToLower(s))
				}
			case map[string]any:
				if name, ok := s["name"].(string); ok {
					name = strings.TrimSpace(name)
					if name != "" {
						out = append(out, strings.ToLower(name))
					}
				}
			}
		}
	case map[string]any:
		// sometimes a map with name
		if name, ok := t["name"].(string); ok {
			name = strings.TrimSpace(name)
			if name != "" {
				out = append(out, strings.ToLower(name))
			}
		}
	case string:
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, strings.ToLower(t))
		}
	}
	// dedupe
	if len(out) == 0 {
		return out
	}
	seen := map[string]bool{}
	uniq := make([]string, 0, len(out))
	for _, k := range out {
		if !seen[k] {
			seen[k] = true
			uniq = append(uniq, k)
		}
	}
	return uniq
}

func extractYear(s string) int {
	s = strings.TrimSpace(s)
	// find 4-digit year
	for i := 0; i+4 <= len(s); i++ {
		part := s[i:]
		for j := 4; j <= len(part); j++ {
			if j < 4 {
				continue
			}
			if len(part) >= 4 {
				if y := tryParseYear(part[:4]); y > 0 {
					return y
				}
			}
			break
		}
		if len(s) < 4 {
			break
		}
	}
	// simple fallback if the string itself is a year
	return tryParseYear(s)
}

func tryParseYear(s string) int {
	if len(s) < 4 {
		return 0
	}
	var y int
	_, err := fmt.Sscanf(s[:4], "%d", &y)
	if err != nil {
		return 0
	}
	if y >= 1000 && y <= time.Now().Year()+1 {
		return y
	}
	return 0
}

func splitName(name string) (family, given string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		return parts[0], ""
	}
	family = parts[len(parts)-1]
	givenNames := parts[:len(parts)-1]
	// Build initials like "F. M."
	var initials []string
	for _, gn := range givenNames {
		if gn == "" {
			continue
		}
		initials = append(initials, strings.ToUpper(string(gn[0]))+".")
	}
	given = strings.Join(initials, " ")
	return family, given
}
