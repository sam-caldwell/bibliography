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

	"bibliography/src/internal/dates"
	"bibliography/src/internal/httpx"
	"bibliography/src/internal/names"
	"bibliography/src/internal/sanitize"
	"bibliography/src/internal/schema"
)

var client httpx.Doer = &http.Client{Timeout: 10 * time.Second}

// SetHTTPClient allows tests to inject a fake HTTP client.
func SetHTTPClient(c httpx.Doer) { client = c }

// FetchBookByISBN queries OpenLibrary and maps the response to schema.Entry.
func FetchBookByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
    norm := normalizeISBN(isbn)
    req := buildOpenLibraryRequest(ctx, norm)
    resp, err := client.Do(req)
    if err != nil { return schema.Entry{}, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return schema.Entry{}, fmt.Errorf("openlibrary: http %d: %s", resp.StatusCode, string(b))
    }
    data, ok, err := decodeOpenLibraryData(resp, norm)
    if err != nil { return schema.Entry{}, err }
    if !ok {
        if e, err := fetchGoogleBookByISBN(ctx, norm); err == nil { return e, nil }
        return schema.Entry{}, fmt.Errorf("openlibrary: no data for ISBN:%s", norm)
    }
    e := mapOpenLibraryToEntry(ctx, data, norm, isbn)
    sanitize.CleanEntry(&e)
    if err := e.Validate(); err != nil { return schema.Entry{}, err }
    return e, nil
}

type olData struct {
    Title       string `json:"title"`
    PublishDate string `json:"publish_date"`
    URL         string `json:"url"`
    Authors     []struct{ Name string } `json:"authors"`
    Publishers  []struct{ Name string } `json:"publishers"`
    Subjects    []struct{ Name string } `json:"subjects"`
}

func buildOpenLibraryRequest(ctx context.Context, norm string) *http.Request {
    q := url.Values{}
    q.Set("bibkeys", "ISBN:"+norm)
    q.Set("format", "json")
    q.Set("jscmd", "data")
    endpoint := "https://openlibrary.org/api/books?" + q.Encode()
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    req.Header.Set("Accept", "application/json")
    httpx.SetUA(req)
    return req
}

func decodeOpenLibraryData(resp *http.Response, norm string) (olData, bool, error) {
    var raw map[string]json.RawMessage
    if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil { return olData{}, false, err }
    key := "ISBN:" + norm
    dataRaw, ok := raw[key]
    if !ok || len(dataRaw) == 0 { return olData{}, false, nil }
    var data olData
    if err := json.Unmarshal(dataRaw, &data); err != nil { return olData{}, false, err }
    return data, true, nil
}

func mapOpenLibraryToEntry(ctx context.Context, data olData, norm string, origISBN string) schema.Entry {
    var e schema.Entry
    e.Type = "book"
    e.APA7.Title = data.Title
    if len(data.Publishers) > 0 { e.APA7.Publisher = data.Publishers[0].Name }
    e.APA7.ISBN = norm
    if strings.TrimSpace(data.URL) != "" { e.APA7.URL = data.URL; e.APA7.Accessed = dates.NowISO() }
    if y := dates.ExtractYear(data.PublishDate); y > 0 { e.APA7.Year = &y }
    for _, a := range data.Authors {
        fam, giv := names.Split(a.Name)
        e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
    }
    for _, s := range data.Subjects {
        name := strings.TrimSpace(s.Name)
        if name != "" { e.Annotation.Keywords = append(e.Annotation.Keywords, strings.ToLower(name)) }
    }
    desc, moreKs := fetchDescriptionFallback(ctx, norm)
    if len(moreKs) > 0 { e.Annotation.Keywords = append(e.Annotation.Keywords, moreKs...) }
    if len(e.Annotation.Keywords) == 0 { e.Annotation.Keywords = []string{"book"} }
    if strings.TrimSpace(desc) != "" {
        e.Annotation.Summary = desc
    } else {
        if e.APA7.Title != "" {
            yearStr := ""
            if e.APA7.Year != nil { yearStr = fmt.Sprintf("%d", *e.APA7.Year) }
            if yearStr != "" && e.APA7.Publisher != "" {
                e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s, %s) from OpenLibrary.", e.APA7.Title, e.APA7.Publisher, yearStr)
            } else if e.APA7.Publisher != "" {
                e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s) from OpenLibrary.", e.APA7.Title, e.APA7.Publisher)
            } else {
                e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from OpenLibrary.", e.APA7.Title)
            }
        } else {
            e.Annotation.Summary = fmt.Sprintf("Bibliographic record from OpenLibrary for ISBN %s.", origISBN)
        }
    }
    if strings.TrimSpace(e.ID) == "" { e.ID = schema.NewID() }
    return e
}

// fetchGoogleBookByISBN queries Google Books API for a given ISBN and maps the first result.
func fetchGoogleBookByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
    req := buildGoogleBooksRequest(ctx, isbn)
    resp, err := client.Do(req)
    if err != nil { return schema.Entry{}, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
        return schema.Entry{}, fmt.Errorf("googlebooks: http %d: %s", resp.StatusCode, string(b))
    }
    gb, err := decodeGoogleBooks(resp)
    if err != nil { return schema.Entry{}, err }
    if len(gb.Items) == 0 { return schema.Entry{}, fmt.Errorf("googlebooks: no items for %s", isbn) }
    e := mapGoogleBookToEntry(gb.Items[0].VolumeInfo, isbn)
    if strings.TrimSpace(e.ID) == "" { e.ID = schema.NewID() }
    sanitize.CleanEntry(&e)
    if err := e.Validate(); err != nil { return schema.Entry{}, err }
    return e, nil
}

type gBooksResp struct {
    Items []struct{ VolumeInfo gVolume `json:"volumeInfo"` } `json:"items"`
}
type gVolume struct {
    Title         string   `json:"title"`
    Authors       []string `json:"authors"`
    Publisher     string   `json:"publisher"`
    PublishedDate string   `json:"publishedDate"`
    Description   string   `json:"description"`
    Categories    []string `json:"categories"`
    InfoLink      string   `json:"infoLink"`
}

func buildGoogleBooksRequest(ctx context.Context, isbn string) *http.Request {
    q := url.Values{}
    q.Set("q", "isbn:"+isbn)
    endpoint := "https://www.googleapis.com/books/v1/volumes?" + q.Encode()
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    req.Header.Set("Accept", "application/json")
    httpx.SetUA(req)
    return req
}

func decodeGoogleBooks(resp *http.Response) (gBooksResp, error) {
    var r gBooksResp
    if err := json.NewDecoder(resp.Body).Decode(&r); err != nil { return gBooksResp{}, err }
    return r, nil
}

func mapGoogleBookToEntry(v gVolume, isbn string) schema.Entry {
    var e schema.Entry
    e.Type = "book"
    e.APA7.Title = v.Title
    e.APA7.Publisher = v.Publisher
    e.APA7.ISBN = isbn
    if y := dates.ExtractYear(v.PublishedDate); y > 0 { e.APA7.Year = &y }
    if strings.TrimSpace(v.InfoLink) != "" { e.APA7.URL = v.InfoLink; e.APA7.Accessed = dates.NowISO() }
    for _, a := range v.Authors {
        fam, giv := names.Split(a)
        e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
    }
    for _, c := range v.Categories {
        c = strings.TrimSpace(c)
        if c != "" { e.Annotation.Keywords = append(e.Annotation.Keywords, strings.ToLower(c)) }
    }
    if len(e.Annotation.Keywords) == 0 { e.Annotation.Keywords = []string{"book"} }
    if strings.TrimSpace(v.Description) != "" {
        e.Annotation.Summary = strings.TrimSpace(v.Description)
    } else if e.APA7.Title != "" {
        if e.APA7.Publisher != "" && e.APA7.Year != nil {
            e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s, %d) from Google Books.", e.APA7.Title, e.APA7.Publisher, *e.APA7.Year)
        } else if e.APA7.Publisher != "" {
            e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s) from Google Books.", e.APA7.Title, e.APA7.Publisher)
        } else {
            e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from Google Books.", e.APA7.Title)
        }
    }
    return e
}

// normalizeISBN cleans input and, if a 9-digit core is provided, computes the ISBN-10 check digit.
func normalizeISBN(isbn string) string {
	s := strings.ToUpper(strings.TrimSpace(isbn))
	// keep digits only initially
	core := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' || r == 'X' {
			core = append(core, r)
		}
	}
	// If 9 digits, compute ISBN-10 check digit
	digitsOnly := true
	for _, r := range core {
		if r < '0' || r > '9' {
			digitsOnly = false
			break
		}
	}
	if len(core) == 9 && digitsOnly {
		cd := isbn10CheckDigit(string(core))
		return string(core) + cd
	}
	return string(core)
}

// isbn10CheckDigit computes the ISBN-10 check digit for a 9-digit string, returning "0"-"9" or "X".
func isbn10CheckDigit(s string) string {
	sum := 0
	for i, ch := range s { // i=0..8 corresponds to position 1..9
		d := int(ch - '0')
		sum += (i + 1) * d
	}
	mod := sum % 11
	cd := (11 - mod) % 11
	if cd == 10 {
		return "X"
	}
	return fmt.Sprintf("%d", cd)
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
	httpx.SetUA(req)
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

// fetchWorkDescription loads a work JSON by key and returns its description text.
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
	httpx.SetUA(req)
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

// toDescription coerces either a string or {value: string} into a description.
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

// parseSubjects extracts lowercase subject names from varied API response shapes.
func parseSubjects(v any) []string {
    seen := map[string]bool{}
    out := []string{}
    add := func(s string) {
        if s == "" || seen[s] { return }
        seen[s] = true
        out = append(out, s)
    }
    switch t := v.(type) {
    case []any:
        for _, it := range t {
            if s := subjectName(it); s != "" { add(s) }
        }
    default:
        if s := subjectName(t); s != "" { add(s) }
    }
    return out
}

func subjectName(v any) string {
    switch t := v.(type) {
    case string:
        s := strings.TrimSpace(t)
        if s == "" { return "" }
        return strings.ToLower(s)
    case map[string]any:
        if name, ok := t["name"].(string); ok {
            name = strings.TrimSpace(name)
            if name != "" { return strings.ToLower(name) }
        }
    }
    return ""
}

// duplicate helpers removed: use dates.ExtractYear and names.Split
