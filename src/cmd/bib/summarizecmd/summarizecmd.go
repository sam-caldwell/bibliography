package summarizecmd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"bibliography/src/internal/httpx"
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bibliography/src/internal/summarize"
)

// New returns the summarize command that fills summaries/keywords via OpenAI.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summarize",
		Short: "Generate summaries and keywords via OpenAI for entries missing a proper summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := store.ReadAll()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			updated := 0
			for _, e := range entries {
				changed, err := processEntry(ctx, cmd, e)
				if err != nil {
					return err
				}
				if changed {
					updated++
				}
			}
			if updated == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no entries needed summaries")
			}
			return nil
		},
	}
	return cmd
}

// Injection seams for OpenAI summarize/keywords to allow faking in tests.
var (
	summarizeURLFunc                = summarize.SummarizeURL
	keywordsFromTitleAndSummaryFunc = summarize.KeywordsFromTitleAndSummary
)

func processEntry(ctx context.Context, cmd *cobra.Command, e schema.Entry) (bool, error) {
	if !needsSummary(e) || strings.TrimSpace(e.APA7.URL) == "" {
		return false, nil
	}
	if !urlAccessible(ctx, e.APA7.URL) {
		fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: url not accessible\n", e.ID)
		return false, nil
	}
	s, err := summarizeURLFunc(ctx, e.APA7.URL)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: %v\n", e.ID, err)
		return false, nil
	}
	e.Annotation.Summary = wrapText(strings.TrimSpace(s), 110)
	if ks, kerr := keywordsFromTitleAndSummaryFunc(ctx, e.APA7.Title, e.Annotation.Summary); kerr == nil {
		e.Annotation.Keywords = mergeSortDedupKeywords(e.Annotation.Keywords, ks, strings.ToLower(e.Type))
	} else {
		if len(e.Annotation.Keywords) == 0 {
			e.Annotation.Keywords = []string{strings.ToLower(e.Type)}
		}
		e.Annotation.Keywords = mergeSortDedupKeywords(e.Annotation.Keywords, nil, "")
	}
	schema.EnsureAccessedIfURL(&e)
	if err := e.Validate(); err != nil {
		return false, nil
	}
	if _, err := store.WriteEntry(e); err != nil {
		return false, err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", e.ID)
	return true, nil
}

func needsSummary(e schema.Entry) bool {
	sum := strings.TrimSpace(e.Annotation.Summary)
	low := strings.ToLower(sum)
	return sum == "" || strings.HasPrefix(low, "bibliographic record") || strings.HasPrefix(low, "ibliographic record")
}

const chromeUA = httpx.ChromeUA

func urlAccessible(ctx context.Context, u string) bool {
	c := &http.Client{Timeout: 10 * time.Second}
	if req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil); err == nil {
		httpx.SetUA(req)
		if resp, err := c.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
		}
	}
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil); err == nil {
		httpx.SetUA(req)
		if resp, err := c.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
		}
	}
	return false
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
