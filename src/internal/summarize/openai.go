package summarize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"bibliography/src/internal/schema"
)

// HTTPDoer allows test injection
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var client HTTPDoer = &http.Client{Timeout: 15 * time.Second}

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

func SetHTTPClient(c HTTPDoer) { client = c }

// SummarizeURL asks OpenAI to produce ~200-word prose summary for a given URL.
func SummarizeURL(ctx context.Context, url string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is not set")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	body := map[string]any{
		"model":       model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a concise scholarly assistant. Write ~200 words of neutral prose suitable for an annotated bibliography. Avoid bullets, quotes, disclaimers."},
			{"role": "user", "content": fmt.Sprintf("Please summarize this work in about 200 words. Use the page itself as reference if you can access it. URL: %s", url)},
		},
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("openai: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

// KeywordsFromTitleAndSummary asks OpenAI for topical keywords given title and summary.
// It returns a list of lowercase keywords. The model is instructed to return ONLY a
// JSON array of strings for robust parsing.
func KeywordsFromTitleAndSummary(ctx context.Context, title, summary string) ([]string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	userPrompt := fmt.Sprintf("Given the following work, return 5-12 topical keywords as a JSON array of lowercase strings. Use single- or short multi-word terms (no sentences), avoid duplicates and punctuation, and do not explain.\\n\\nTitle: %s\\nSummary: %s\\n\\nReturn ONLY a JSON array, e.g., [\"keyword\", \"another\"].", title, summary)

	body := map[string]any{
		"model":       model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": "You generate concise topical keywords for cataloging and search. Output strictly JSON arrays of lowercase strings."},
			{"role": "user", "content": userPrompt},
		},
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("openai: empty choices")
	}
	content := out.Choices[0].Message.Content

	// Try strict JSON array parsing first
	var arr []string
	if err := json.Unmarshal([]byte(content), &arr); err == nil {
		cleaned := make([]string, 0, len(arr))
		for _, k := range arr {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			cleaned = append(cleaned, strings.ToLower(k))
		}
		return cleaned, nil
	}

	// Fallback: attempt to find a JSON array within content
	// This is a best-effort extraction to be resilient to minor formatting drift.
	start := -1
	end := -1
	for i, r := range content {
		if r == '[' {
			start = i
			break
		}
	}
	for i := len(content) - 1; i >= 0; i-- {
		if content[i] == ']' {
			end = i + 1
			break
		}
	}
	if start >= 0 && end > start {
		snippet := content[start:end]
		if err := json.Unmarshal([]byte(snippet), &arr); err == nil {
			cleaned := make([]string, 0, len(arr))
			for _, k := range arr {
				k = strings.TrimSpace(k)
				if k == "" {
					continue
				}
				cleaned = append(cleaned, strings.ToLower(k))
			}
			return cleaned, nil
		}
	}

	// Last resort: split by comma and clean heuristically
	// This path tries to salvage something rather than fail the whole command.
	parts := strings.Split(content, ",")
	if len(parts) == 1 && strings.Contains(content, "\n") {
		parts = strings.Split(content, "\n")
	}
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "[]\"') ")
		if p == "" {
			continue
		}
		cleaned = append(cleaned, strings.ToLower(p))
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("openai: could not parse keywords")
	}
	return cleaned, nil
}

// GenerateMovieFromTitleAndDate asks OpenAI to return a minimal JSON object describing a film and
// builds a schema.Entry of type "movie". It also asks for a short neutral summary and derives
// keywords from title and summary. Requires OPENAI_API_KEY.
func GenerateMovieFromTitleAndDate(ctx context.Context, title, date string) (schema.Entry, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return schema.Entry{}, fmt.Errorf("OPENAI_API_KEY is not set")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	sys := "You extract bibliographic metadata for films. Return strict JSON only."
	user := fmt.Sprintf(`Given this film information, return ONLY a single JSON object with keys:
{
  "title": string,
  "date": string,               // YYYY-MM-DD if known; else empty
  "publisher": string,          // studio or distributor; may be empty
  "authors": [{"family": string, "given": string}] ,  // directors; may be empty
  "summary": string             // 120-200 word neutral prose
}
Title: %s
Date: %s`, title, date)
	body := map[string]any{
		"model":       model,
		"temperature": 0.2,
		"messages":    []map[string]string{{"role": "system", "content": sys}, {"role": "user", "content": user}},
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("openai: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Choices) == 0 {
		return schema.Entry{}, fmt.Errorf("openai: empty choices")
	}
	content := out.Choices[0].Message.Content
	var obj struct {
		Title     string `json:"title"`
		Date      string `json:"date"`
		Publisher string `json:"publisher"`
		Authors   []struct{ Family, Given string }
		Summary   string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		// Try to recover a JSON object from content
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			snippet := content[start : end+1]
			_ = json.Unmarshal([]byte(snippet), &obj)
		}
	}
	e := schema.Entry{Type: "movie"}
	e.ID = schema.NewID()
	if strings.TrimSpace(obj.Title) != "" {
		e.APA7.Title = strings.TrimSpace(obj.Title)
	} else {
		e.APA7.Title = title
	}
	e.APA7.Date = strings.TrimSpace(obj.Date)
	if e.APA7.Date == "" {
		e.APA7.Date = strings.TrimSpace(date)
	}
	if y := yearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.Publisher = strings.TrimSpace(obj.Publisher)
	for _, a := range obj.Authors {
		fam := strings.TrimSpace(a.Family)
		giv := strings.TrimSpace(a.Given)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	sum := strings.TrimSpace(obj.Summary)
	if sum == "" && e.APA7.Title != "" {
		sum = fmt.Sprintf("Film: %s.", e.APA7.Title)
	}
	e.Annotation.Summary = sum
	if ks, err := KeywordsFromTitleAndSummary(ctx, e.APA7.Title, e.Annotation.Summary); err == nil {
		cleaned := make([]string, 0, len(ks))
		seen := map[string]bool{}
		for _, k := range ks {
			k = strings.ToLower(strings.TrimSpace(k))
			if k != "" && !seen[k] {
				seen[k] = true
				cleaned = append(cleaned, k)
			}
		}
		if len(cleaned) > 0 {
			e.Annotation.Keywords = cleaned
		}
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{"movie"}
	}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// GenerateCitationFromURL asks OpenAI to produce minimal bibliographic metadata
// for an article reachable at the given URL. It returns a schema.Entry with type
// "article" filled with best-effort fields. It also attempts to generate a summary
// and keywords using the existing helpers. Requires OPENAI_API_KEY.
func GenerateCitationFromURL(ctx context.Context, url string) (schema.Entry, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return schema.Entry{}, fmt.Errorf("OPENAI_API_KEY is not set")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	sys := "You extract bibliographic metadata for an online article."
	user := fmt.Sprintf(`Given this URL, return ONLY a single JSON object with these keys:
{
  "title": string,
  "authors": [{"family": string, "given": string}] ,
  "journal": string,            // publication/container title if known; else empty
  "container_title": string,    // alternative to journal; may be empty
  "publisher": string,          // website or publisher; may be empty
  "date": string,               // YYYY-MM-DD if known; else empty
  "doi": string                 // DOI if known; else empty
}
Use the page if accessible; otherwise use general knowledge cautiously. If unknown, use empty strings.
URL: %s`, url)

	body := map[string]any{
		"model":       model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": user},
		},
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("openai: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Choices) == 0 {
		return schema.Entry{}, fmt.Errorf("openai: empty choices")
	}
	content := out.Choices[0].Message.Content
	var obj struct {
		Title          string                           `json:"title"`
		Authors        []struct{ Family, Given string } `json:"authors"`
		Journal        string                           `json:"journal"`
		ContainerTitle string                           `json:"container_title"`
		Publisher      string                           `json:"publisher"`
		Date           string                           `json:"date"`
		DOI            string                           `json:"doi"`
	}
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			snippet := content[start : end+1]
			_ = json.Unmarshal([]byte(snippet), &obj)
		}
	}
	e := schema.Entry{Type: "article"}
	e.ID = schema.NewID()
	e.APA7.Title = strings.TrimSpace(obj.Title)
	if e.APA7.Title == "" {
		e.APA7.Title = url
	}
	if j := strings.TrimSpace(obj.Journal); j != "" {
		e.APA7.Journal = j
		e.APA7.ContainerTitle = j
	}
	if c := strings.TrimSpace(obj.ContainerTitle); c != "" && e.APA7.Journal == "" {
		e.APA7.ContainerTitle = c
		e.APA7.Journal = c
	}
	e.APA7.Publisher = strings.TrimSpace(obj.Publisher)
	e.APA7.Date = strings.TrimSpace(obj.Date)
	if y := yearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.DOI = strings.TrimSpace(obj.DOI)
	e.APA7.URL = url
	e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	for _, a := range obj.Authors {
		fam := strings.TrimSpace(a.Family)
		giv := strings.TrimSpace(a.Given)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	if s, err := SummarizeURL(ctx, url); err == nil {
		e.Annotation.Summary = strings.TrimSpace(s)
	}
	if strings.TrimSpace(e.Annotation.Summary) == "" && e.APA7.Title != "" {
		e.Annotation.Summary = fmt.Sprintf("Web article: %s.", e.APA7.Title)
	}
	if ks, err := KeywordsFromTitleAndSummary(ctx, e.APA7.Title, e.Annotation.Summary); err == nil {
		cleaned := make([]string, 0, len(ks))
		seen := map[string]bool{}
		for _, k := range ks {
			k = strings.ToLower(strings.TrimSpace(k))
			if k != "" && !seen[k] {
				seen[k] = true
				cleaned = append(cleaned, k)
			}
		}
		if len(cleaned) > 0 {
			e.Annotation.Keywords = cleaned
		}
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{"article"}
	}
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

// GenerateSongFromTitleArtistDate builds a minimal APA7 song entry from OpenAI output.
func GenerateSongFromTitleArtistDate(ctx context.Context, title, artist, date string) (schema.Entry, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return schema.Entry{}, fmt.Errorf("OPENAI_API_KEY is not set")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	sys := "You extract bibliographic metadata for songs. Return strict JSON only."
	user := fmt.Sprintf(`Given this song info, return ONLY a single JSON object with keys:
{
  "title": string,
  "date": string,               // YYYY-MM-DD if known; else empty
  "publisher": string,          // label if known; else empty
  "container_title": string,    // album if known; else empty
  "authors": [{"family": string, "given": string}] ,  // artist/performer; may be empty
  "summary": string             // 80-160 word neutral prose
}
Title: %s
Artist: %s
Date: %s`, title, artist, date)
	body := map[string]any{"model": model, "temperature": 0.2, "messages": []map[string]string{{"role": "system", "content": sys}, {"role": "user", "content": user}}}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", ua)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("openai: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Choices) == 0 {
		return schema.Entry{}, fmt.Errorf("openai: empty choices")
	}
	content := out.Choices[0].Message.Content
	var obj struct {
		Title, Date, Publisher, ContainerTitle, Summary string
		Authors                                         []struct{ Family, Given string }
	}
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		if s2, e := extractJSONObject(content); e == nil {
			_ = json.Unmarshal([]byte(s2), &obj)
		}
	}
	var e schema.Entry
	e.Type = "song"
	e.ID = schema.NewID()
	e.APA7.Title = strings.TrimSpace(first(obj.Title, title))
	e.APA7.Date = strings.TrimSpace(first(obj.Date, date))
	if y := yearFromDate(e.APA7.Date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.Publisher = strings.TrimSpace(obj.Publisher)
	e.APA7.ContainerTitle = strings.TrimSpace(obj.ContainerTitle)
	for _, a := range obj.Authors {
		fam := strings.TrimSpace(a.Family)
		giv := strings.TrimSpace(a.Given)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	e.Annotation.Summary = strings.TrimSpace(obj.Summary)
	if e.Annotation.Summary == "" && e.APA7.Title != "" {
		e.Annotation.Summary = "Song: " + e.APA7.Title + "."
	}
	if ks, err := KeywordsFromTitleAndSummary(ctx, e.APA7.Title, e.Annotation.Summary); err == nil {
		e.Annotation.Keywords = ks
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{"song"}
	}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func first(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
func extractJSONObject(s string) (string, error) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1], nil
	}
	return "", fmt.Errorf("no json object")
}
