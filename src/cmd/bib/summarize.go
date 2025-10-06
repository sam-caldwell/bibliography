package main

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bibliography/src/internal/summarize"
)

func newSummarizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summarize",
		Short: "Generate summaries and keywords via OpenAI for entries missing a proper summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Walk all YAML files under data/citations
			var paths []string
			err := filepath.WalkDir(store.CitationsDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
					return nil
				}
				paths = append(paths, path)
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			ctx := cmd.Context()
			updated := 0
			for _, p := range paths {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				var e schema.Entry
				if err := yaml.Unmarshal(b, &e); err != nil {
					continue
				}
				// Determine if summary needs update
				sum := strings.TrimSpace(e.Annotation.Summary)
				low := strings.ToLower(sum)
				needs := sum == "" || strings.HasPrefix(low, "bibliographic record") || strings.HasPrefix(low, "ibliographic record")
				if !needs {
					continue
				}
				if strings.TrimSpace(e.APA7.URL) == "" {
					continue
				}
				// Ensure the source material is accessible; otherwise do not alter summary
				if !urlAccessible(ctx, e.APA7.URL) {
					fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: url not accessible\n", p)
					continue
				}
				// Call OpenAI for summary
				s, err := summarize.SummarizeURL(ctx, e.APA7.URL)
				if err != nil {
					// print and continue
					fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: %v\n", p, err)
					continue
				}
				s = wrapText(strings.TrimSpace(s), 110)
				e.Annotation.Summary = s
				// Call OpenAI for keywords using title and summary
				if ks, kerr := summarize.KeywordsFromTitleAndSummary(ctx, e.APA7.Title, e.Annotation.Summary); kerr == nil {
					e.Annotation.Keywords = mergeSortDedupKeywords(e.Annotation.Keywords, ks, strings.ToLower(e.Type))
				} else {
					// Fallback: ensure at least the type keyword exists
					if len(e.Annotation.Keywords) == 0 {
						e.Annotation.Keywords = []string{strings.ToLower(e.Type)}
					}
					e.Annotation.Keywords = mergeSortDedupKeywords(e.Annotation.Keywords, nil, "")
				}
				// Ensure accessed if URL present
				if strings.TrimSpace(e.APA7.URL) != "" && strings.TrimSpace(e.APA7.Accessed) == "" {
					e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
				}
				// Validate and write via store
				if err := e.Validate(); err != nil {
					continue
				}
				if _, err := store.WriteEntry(e); err != nil {
					return err
				}
				updated++
				fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", p)
			}
			if updated == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no entries needed summaries")
			}
			return nil
		},
	}
	return cmd
}

func wrapText(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) <= width {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	lines = append(lines, cur)
	return strings.Join(lines, "\n")
}

const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

func urlAccessible(ctx context.Context, u string) bool {
	c := &http.Client{Timeout: 10 * time.Second}
	if req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil); err == nil {
		req.Header.Set("User-Agent", chromeUA)
		if resp, err := c.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
		}
	}
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil); err == nil {
		req.Header.Set("User-Agent", chromeUA)
		if resp, err := c.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
		}
	}
	return false
}

// mergeSortDedupKeywords merges existing and new keywords, optionally including an extra
// keyword (e.g., the entry type), normalizes to lowercase, de-duplicates case-insensitively,
// sorts lexicographically, and returns the final list.
func mergeSortDedupKeywords(existing, generated []string, optional string) []string {
	set := make(map[string]struct{}, len(existing)+len(generated)+1)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		s = strings.ToLower(s)
		set[s] = struct{}{}
	}
	for _, k := range existing {
		add(k)
	}
	for _, k := range generated {
		add(k)
	}
	add(optional)
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
