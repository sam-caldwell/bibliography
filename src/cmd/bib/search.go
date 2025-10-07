package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
)

func newSearchCmd() *cobra.Command {
	var keywords, authorQ, titleQ, summaryQ, allQ string
	cmd := &cobra.Command{
		Use:   "search [expr]",
		Short: "Search citations by keyword/author/title/summary or full record (expr or flags)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := store.ReadAll()
			if err != nil {
				return err
			}

			// Expression mode: bib search "author==kim* && keywords in (k1,k2) && date > 1983"
			if len(args) > 0 {
				expr := strings.Join(args, " ")
				preds, perr := parseExpr(expr)
				if perr != nil {
					return perr
				}
				type scored struct {
					e schema.Entry
					s int
				}
				var out []scored
				for _, e := range entries {
					score := 0
					ok := true
					for _, p := range preds {
						hit, sc := p(e)
						if !hit {
							ok = false
							break
						}
						score += sc
					}
					if ok {
						out = append(out, scored{e: e, s: score})
					}
				}
				sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
				rows := make([][]string, 0, len(out))
				for _, it := range out {
					rows = append(rows, []string{it.e.ID, it.e.Type, it.e.APA7.Title, firstAuthor(it.e)})
				}
				renderTable(cmd.OutOrStdout(), []string{"id", "type", "title", "author"}, rows)
				return nil
			}

			// Legacy keyword-only path with optional field flags
			if strings.TrimSpace(authorQ) == "" && strings.TrimSpace(titleQ) == "" && strings.TrimSpace(summaryQ) == "" && strings.TrimSpace(allQ) == "" {
				if strings.TrimSpace(keywords) == "" {
					return fmt.Errorf("provide an expression, --keyword, or a query flag like --all, --author, --title, or --summary")
				}
				// Relevance using keyword-only scoring
				type scored struct {
					e schema.Entry
					s int
				}
				var out []scored
				for _, e := range entries {
					s := scoreEntry(e, keywords, "", "", "", "")
					if s > 0 {
						out = append(out, scored{e: e, s: s})
					}
				}
				sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
				rows := make([][]string, 0, len(out))
				for _, it := range out {
					rows = append(rows, []string{it.e.ID, it.e.Type, it.e.APA7.Title, firstAuthor(it.e)})
				}
				renderTable(cmd.OutOrStdout(), []string{"id", "type", "title", "author"}, rows)
				return nil
			}

			// Relevance-ranked across requested flag fields
			type scored struct {
				e schema.Entry
				s int
			}
			var out []scored
			for _, e := range entries {
				s := scoreEntry(e, keywords, authorQ, titleQ, summaryQ, allQ)
				if s > 0 {
					out = append(out, scored{e: e, s: s})
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
			rows := make([][]string, 0, len(out))
			for _, it := range out {
				rows = append(rows, []string{it.e.ID, it.e.Type, it.e.APA7.Title, firstAuthor(it.e)})
			}
			renderTable(cmd.OutOrStdout(), []string{"id", "type", "title", "author"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&keywords, "keyword", "", "comma-delimited keywords (AND filter; boosts relevance)")
	cmd.Flags().StringVar(&authorQ, "author", "", "author search (matches family,given)")
	cmd.Flags().StringVar(&titleQ, "title", "", "title full-text search")
	cmd.Flags().StringVar(&summaryQ, "summary", "", "summary full-text search")
	cmd.Flags().StringVar(&allQ, "all", "", "full-record search (YAML)")
	return cmd
}

func firstAuthor(e schema.Entry) string {
	if len(e.APA7.Authors) == 0 {
		return ""
	}
	a := e.APA7.Authors[0]
	fam := strings.TrimSpace(a.Family)
	giv := strings.TrimSpace(a.Given)
	if fam == "" {
		return giv
	}
	if giv == "" {
		return fam
	}
	return fam + ", " + giv
}

// parseExpr supports a minimal boolean AND query language with:
//
//	author==pattern*            (wildcard '*' matches any suffix)
//	keywords in (k1,k2,...)     (any match)
//	date|year <op> YYYY         (op: >, >=, <, <=, ==)
//	title~=text, summary~=text, all~=text (contains)
//
// Terms are combined with '&&'. Whitespace-insensitive. Case-insensitive matching.
func parseExpr(expr string) ([]func(schema.Entry) (bool, int), error) {
	parts := splitAnd(expr)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty expr")
	}
	var preds []func(schema.Entry) (bool, int)
	for _, t := range parts {
		tt := strings.TrimSpace(t)
		if tt == "" {
			continue
		}
		// keywords in (...)
		if m := regexp.MustCompile(`(?i)^keywords\s+in\s*\(([^)]*)\)$`).FindStringSubmatch(tt); m != nil {
			items := splitCSV(m[1])
			preds = append(preds, (func(items []string) func(schema.Entry) (bool, int) {
				set := make([]string, 0, len(items))
				for _, it := range items {
					it = strings.ToLower(strings.TrimSpace(it))
					if it != "" {
						set = append(set, it)
					}
				}
				return func(e schema.Entry) (bool, int) {
					hit := 0
					have := map[string]bool{}
					for _, k := range e.Annotation.Keywords {
						have[strings.ToLower(strings.TrimSpace(k))] = true
					}
					for _, it := range set {
						if have[it] {
							hit++
						}
					}
					if hit == 0 {
						return false, 0
					}
					return true, hit * 5
				}
			})(items))
			continue
		}
		// author==pattern
		if m := regexp.MustCompile(`(?i)^author\s*==\s*([^\s]+)$`).FindStringSubmatch(tt); m != nil {
			pat := strings.ToLower(strings.TrimSpace(m[1]))
			rx := wildcardToRegex(pat)
			preds = append(preds, func(e schema.Entry) (bool, int) {
				for _, a := range e.APA7.Authors {
					name := strings.ToLower(strings.TrimSpace(a.Family))
					if a.Given != "" {
						name += ", " + strings.ToLower(strings.TrimSpace(a.Given))
					}
					if rx.MatchString(name) {
						return true, 7
					}
				}
				return false, 0
			})
			continue
		}
		// year/date comparisons
		if m := regexp.MustCompile(`(?i)^(year|date)\s*(==|>=|<=|>|<)\s*(\d{4})$`).FindStringSubmatch(tt); m != nil {
			op := m[2]
			yv, _ := strconv.Atoi(m[3])
			preds = append(preds, func(e schema.Entry) (bool, int) {
				y := 0
				if e.APA7.Year != nil {
					y = *e.APA7.Year
				} else if len(strings.TrimSpace(e.APA7.Date)) >= 4 {
					var yy int
					fmt.Sscanf(e.APA7.Date[:4], "%d", &yy)
					y = yy
				}
				if y == 0 {
					return false, 0
				}
				ok := false
				switch op {
				case ">":
					ok = y > yv
				case ">=":
					ok = y >= yv
				case "<":
					ok = y < yv
				case "<=":
					ok = y <= yv
				case "==":
					ok = y == yv
				}
				if !ok {
					return false, 0
				}
				return true, 1
			})
			continue
		}
		// title~=, summary~=, all~=
		if m := regexp.MustCompile(`(?i)^(title|summary|all)\s*~=\s*(.+)$`).FindStringSubmatch(tt); m != nil {
			field := strings.ToLower(m[1])
			q := strings.ToLower(strings.TrimSpace(trimQuotes(m[2])))
			preds = append(preds, func(e schema.Entry) (bool, int) {
				switch field {
				case "title":
					c := countContains(strings.ToLower(e.APA7.Title), q)
					if c == 0 {
						return false, 0
					}
					return true, c * 3
				case "summary":
					c := countContains(strings.ToLower(e.Annotation.Summary), q)
					if c == 0 {
						return false, 0
					}
					return true, c * 2
				case "all":
					var buf bytes.Buffer
					_ = yaml.NewEncoder(&buf).Encode(e)
					c := countContains(strings.ToLower(buf.String()), q)
					if c == 0 {
						return false, 0
					}
					return true, c
				}
				return false, 0
			})
			continue
		}
		return nil, fmt.Errorf("unsupported term: %q", tt)
	}
	return preds, nil
}

func splitAnd(expr string) []string {
	// split on '&&' while keeping terms simple (no nested parentheses support yet)
	var parts []string
	for _, p := range strings.Split(expr, "&&") {
		parts = append(parts, strings.TrimSpace(p))
	}
	return parts
}

func splitCSV(s string) []string {
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

func wildcardToRegex(pat string) *regexp.Regexp {
	// Convert '*' wildcards to ".*" while escaping other regex meta characters.
	var b strings.Builder
	for i := 0; i < len(pat); i++ {
		ch := pat[i]
		if ch == '*' {
			b.WriteString(".*")
		} else {
			b.WriteString(regexp.QuoteMeta(string([]byte{ch})))
		}
	}
	rx := "^" + b.String() + "$"
	return regexp.MustCompile(rx)
}

func scoreEntry(e schema.Entry, kwCSV, authorQ, titleQ, summaryQ, allQ string) int {
	s := 0
	if strings.TrimSpace(kwCSV) != "" {
		want := strings.Split(kwCSV, ",")
		set := map[string]bool{}
		for _, k := range e.Annotation.Keywords {
			set[strings.ToLower(strings.TrimSpace(k))] = true
		}
		for _, w := range want {
			w2 := strings.ToLower(strings.TrimSpace(w))
			if w2 == "" {
				continue
			}
			if !set[w2] {
				return 0
			}
			s += 5
		}
	}
	if q := strings.ToLower(strings.TrimSpace(authorQ)); q != "" {
		hit := false
		for _, a := range e.APA7.Authors {
			name := strings.ToLower(strings.TrimSpace(a.Family + ", " + a.Given))
			if strings.Contains(name, q) {
				s += 5
				hit = true
			}
		}
		if !hit {
			return 0
		}
	}
	if q := strings.ToLower(strings.TrimSpace(titleQ)); q != "" {
		add := countContains(strings.ToLower(e.APA7.Title), q) * 3
		if add == 0 {
			return 0
		}
		s += add
	}
	if q := strings.ToLower(strings.TrimSpace(summaryQ)); q != "" {
		add := countContains(strings.ToLower(e.Annotation.Summary), q) * 2
		if add == 0 {
			return 0
		}
		s += add
	}
	if q := strings.ToLower(strings.TrimSpace(allQ)); q != "" {
		var buf bytes.Buffer
		_ = yaml.NewEncoder(&buf).Encode(e)
		add := countContains(strings.ToLower(buf.String()), q)
		if add == 0 {
			return 0
		}
		s += add
	}
	if s == 0 && strings.TrimSpace(kwCSV) != "" {
		s = 1
	}
	return s
}

func countContains(text, q string) int {
	if q == "" {
		return 0
	}
	terms := strings.Fields(q)
	score := 0
	for _, t := range terms {
		if t == "" {
			continue
		}
		idx := 0
		for {
			i := strings.Index(text[idx:], t)
			if i < 0 {
				break
			}
			score++
			idx += i + len(t)
		}
	}
	return score
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// renderTable prints a simple ASCII table (left-aligned) with headers and rows.
func renderTable(w io.Writer, headers []string, rows [][]string) {
	// compute widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i := range headers {
			if i < len(r) {
				if l := len(r[i]); l > widths[i] {
					widths[i] = l
				}
			}
		}
	}
	// header
	for i, h := range headers {
		fmt.Fprintf(w, "%-*s", widths[i], h)
		if i != len(headers)-1 {
			fmt.Fprint(w, "  ")
		}
	}
	fmt.Fprint(w, "\n")
	// separator
	for i := range headers {
		fmt.Fprint(w, strings.Repeat("-", widths[i]))
		if i != len(headers)-1 {
			fmt.Fprint(w, "  ")
		}
	}
	fmt.Fprint(w, "\n")
	// rows
	for _, r := range rows {
		for i := range headers {
			val := ""
			if i < len(r) {
				val = r[i]
			}
			fmt.Fprintf(w, "%-*s", widths[i], val)
			if i != len(headers)-1 {
				fmt.Fprint(w, "  ")
			}
		}
		fmt.Fprint(w, "\n")
	}
}
