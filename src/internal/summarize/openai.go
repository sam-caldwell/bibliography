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
