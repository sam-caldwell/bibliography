package searchcmd

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

// New returns the search command for keyword and expression-based querying.
func New() *cobra.Command {
    var keywords, authorQ, titleQ, summaryQ, allQ string
    cmd := &cobra.Command{
        Use:   "search [expr]",
        Short: "Search citations by keyword/author/title/summary or full record (expr or flags)",
        Args:  cobra.ArbitraryArgs,
        RunE: func(cmd *cobra.Command, args []string) error {
            entries, err := store.ReadAll(); if err != nil { return err }
            if len(args) > 0 { return runExprSearch(cmd, entries, strings.Join(args, " ")) }
            if isEmpty(authorQ) && isEmpty(titleQ) && isEmpty(summaryQ) && isEmpty(allQ) {
                if isEmpty(keywords) { return fmt.Errorf("provide an expression, --keyword, or a query flag like --all, --author, --title, or --summary") }
                return runKeywordOnlySearch(cmd, entries, keywords)
            }
            return runFlagSearch(cmd, entries, keywords, authorQ, titleQ, summaryQ, allQ)
        },
    }
    cmd.Flags().StringVar(&keywords, "keyword", "", "comma-delimited keywords (AND filter; boosts relevance)")
    cmd.Flags().StringVar(&authorQ, "author", "", "author search (matches family,given)")
    cmd.Flags().StringVar(&titleQ, "title", "", "title full-text search")
    cmd.Flags().StringVar(&summaryQ, "summary", "", "summary full-text search")
    cmd.Flags().StringVar(&allQ, "all", "", "full-record search (YAML)")
    return cmd
}

func isEmpty(s string) bool { return strings.TrimSpace(s) == "" }

type scored struct { e schema.Entry; s int }

func runExprSearch(cmd *cobra.Command, entries []schema.Entry, expr string) error {
    preds, err := parseExpr(expr); if err != nil { return err }
    var out []scored
    for _, e := range entries {
        score := 0
        ok := true
        for _, p := range preds {
            hit, sc := p(e)
            if !hit { ok = false; break }
            score += sc
        }
        if ok { out = append(out, scored{e: e, s: score}) }
    }
    sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
    renderResults(cmd, out)
    return nil
}

func runKeywordOnlySearch(cmd *cobra.Command, entries []schema.Entry, keywords string) error {
    var out []scored
    for _, e := range entries { s := scoreEntry(e, keywords, "", "", "", ""); if s > 0 { out = append(out, scored{e: e, s: s}) } }
    sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
    renderResults(cmd, out)
    return nil
}

func runFlagSearch(cmd *cobra.Command, entries []schema.Entry, keywords, authorQ, titleQ, summaryQ, allQ string) error {
    var out []scored
    for _, e := range entries { s := scoreEntry(e, keywords, authorQ, titleQ, summaryQ, allQ); if s > 0 { out = append(out, scored{e: e, s: s}) } }
    sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
    renderResults(cmd, out)
    return nil
}

func renderResults(cmd *cobra.Command, out []scored) {
    rows := make([][]string, 0, len(out))
    for _, it := range out { rows = append(rows, []string{it.e.ID, it.e.Type, it.e.APA7.Title, firstAuthor(it.e)}) }
    renderTable(cmd.OutOrStdout(), []string{"id", "type", "title", "author"}, rows)
}

func firstAuthor(e schema.Entry) string { if len(e.APA7.Authors)==0 { return "" }; a:=e.APA7.Authors[0]; fam:=strings.TrimSpace(a.Family); giv:=strings.TrimSpace(a.Given); if fam=="" { return giv }; if giv=="" { return fam }; return fam+", "+giv }

type predicate func(schema.Entry) (hit bool, score int)

func parseExpr(expr string) ([]predicate, error) {
    if strings.TrimSpace(expr) == "" { return nil, fmt.Errorf("empty expression") }
    terms := splitAnd(expr)
    var preds []predicate
    for _, tt := range terms {
        if p, ok, err := compileKeywordTerm(tt); err != nil { return nil, err } else if ok { preds = append(preds, p); continue }
        if p, ok, err := compileAuthorEqualsTerm(tt); err != nil { return nil, err } else if ok { preds = append(preds, p); continue }
        if p, ok, err := compileDateCompareTerm(tt); err != nil { return nil, err } else if ok { preds = append(preds, p); continue }
        if p, ok, err := compileContainsTerm(tt); err != nil { return nil, err } else if ok { preds = append(preds, p); continue }
        return nil, fmt.Errorf("unrecognized term: %s", tt)
    }
    return preds, nil
}

func compileKeywordTerm(tt string) (predicate, bool, error) {
    m := regexp.MustCompile(`(?i)^keyword\s*==\s*(.+)$`).FindStringSubmatch(tt)
    if m == nil { return nil, false, nil }
    items := splitCSV(m[1]); if len(items)==0 { return nil, false, fmt.Errorf("empty keywords") }
    set := map[string]bool{}; for _, it := range items { set[strings.ToLower(it)] = true }
    p := func(e schema.Entry) (bool, int) {
        hit := 0
        have := map[string]bool{}
        for _, k := range e.Annotation.Keywords { have[strings.ToLower(strings.TrimSpace(k))] = true }
        for _, it := range set { _ = it }
        for k := range set { if have[k] { hit++ } }
        if hit == 0 { return false, 0 }
        return true, hit * 5
    }
    return p, true, nil
}

func compileAuthorEqualsTerm(tt string) (predicate, bool, error) {
    m := regexp.MustCompile(`(?i)^author\s*==\s*([^\s]+)$`).FindStringSubmatch(tt)
    if m == nil { return nil, false, nil }
    pat := strings.ToLower(strings.TrimSpace(m[1]))
    rx := WildcardToRegex(pat)
    p := func(e schema.Entry) (bool, int) {
        for _, a := range e.APA7.Authors {
            name := strings.ToLower(strings.TrimSpace(a.Family))
            if a.Given != "" { name += ", " + strings.ToLower(strings.TrimSpace(a.Given)) }
            if rx.MatchString(name) { return true, 7 }
        }
        return false, 0
    }
    return p, true, nil
}

func compileDateCompareTerm(tt string) (predicate, bool, error) {
    m := regexp.MustCompile(`(?i)^(year|date)\s*(==|>=|<=|>|<)\s*(\d{4})$`).FindStringSubmatch(tt)
    if m == nil { return nil, false, nil }
    op := m[2]; yv, _ := strconv.Atoi(m[3])
    p := func(e schema.Entry) (bool, int) {
        y := 0
        if e.APA7.Year != nil { y = *e.APA7.Year } else if len(strings.TrimSpace(e.APA7.Date)) >= 4 { var yy int; if _, err := fmt.Sscanf(e.APA7.Date[:4], "%d", &yy); err != nil { return false, 0 }; y = yy }
        if y == 0 { return false, 0 }
        ok := false
        switch op { case ">": ok = y > yv; case ">=": ok = y >= yv; case "<": ok = y < yv; case "<=": ok = y <= yv; case "==": ok = y == yv }
        if !ok { return false, 0 }
        return true, 1
    }
    return p, true, nil
}

func compileContainsTerm(tt string) (predicate, bool, error) {
    m := regexp.MustCompile(`(?i)^(title|summary|all)\s*~=\s*(.+)$`).FindStringSubmatch(tt)
    if m == nil { return nil, false, nil }
    field := strings.ToLower(m[1]); q := strings.ToLower(strings.TrimSpace(trimQuotes(m[2])))
    p := func(e schema.Entry) (bool, int) {
        switch field {
        case "title":
            c := CountContains(strings.ToLower(e.APA7.Title), q); if c == 0 { return false, 0 }; return true, c * 3
        case "summary":
            c := CountContains(strings.ToLower(e.Annotation.Summary), q); if c == 0 { return false, 0 }; return true, c * 2
        case "all":
            var buf bytes.Buffer; _ = yaml.NewEncoder(&buf).Encode(e); c := CountContains(strings.ToLower(buf.String()), q); if c == 0 { return false, 0 }; return true, c
        }
        return false, 0
    }
    return p, true, nil
}

func splitAnd(expr string) []string { var parts []string; for _, p := range strings.Split(expr, "&&") { parts = append(parts, strings.TrimSpace(p)) }; return parts }
func splitCSV(s string) []string { parts := strings.Split(s, ","); out := make([]string, 0, len(parts)); for _, p := range parts { p = strings.TrimSpace(p); if p != "" { out = append(out, p) } }; return out }
func WildcardToRegex(pat string) *regexp.Regexp { var b strings.Builder; for i:=0;i<len(pat);i++ { ch := pat[i]; if ch=='*' { b.WriteString(".*") } else { b.WriteString(regexp.QuoteMeta(string([]byte{ch}))) } }; rx := "^"+b.String()+"$"; return regexp.MustCompile(rx) }

func scoreEntry(e schema.Entry, kwCSV, authorQ, titleQ, summaryQ, allQ string) int {
    s := 0
    if add, ok := scoreKeywords(e, kwCSV); !ok { return 0 } else { s += add }
    if add, ok := scoreAuthor(e, authorQ); !ok { return 0 } else { s += add }
    if add, ok := scoreTitle(e, titleQ); !ok { return 0 } else { s += add }
    if add, ok := scoreSummary(e, summaryQ); !ok { return 0 } else { s += add }
    if add, ok := scoreAll(e, allQ); !ok { return 0 } else { s += add }
    if s == 0 && strings.TrimSpace(kwCSV) != "" { s = 1 }
    return s
}

func scoreKeywords(e schema.Entry, kwCSV string) (int, bool) {
    if strings.TrimSpace(kwCSV) == "" { return 0, true }
    want := strings.Split(kwCSV, ","); set := map[string]bool{}
    for _, k := range e.Annotation.Keywords { set[strings.ToLower(strings.TrimSpace(k))] = true }
    s := 0
    for _, w := range want { w2 := strings.ToLower(strings.TrimSpace(w)); if w2 == "" { continue }; if !set[w2] { return 0, false }; s += 5 }
    return s, true
}
func scoreAuthor(e schema.Entry, q string) (int, bool) { q=strings.ToLower(strings.TrimSpace(q)); if q=="" { return 0, true }; hit:=false; s:=0; for _, a := range e.APA7.Authors { name:=strings.ToLower(strings.TrimSpace(a.Family+", "+a.Given)); if strings.Contains(name,q){ s+=5; hit=true } }; if !hit { return 0, false }; return s, true }
func scoreTitle(e schema.Entry, q string) (int, bool) { q=strings.ToLower(strings.TrimSpace(q)); if q=="" { return 0, true }; add:=CountContains(strings.ToLower(e.APA7.Title), q)*3; if add==0 { return 0, false }; return add, true }
func scoreSummary(e schema.Entry, q string) (int, bool) { q=strings.ToLower(strings.TrimSpace(q)); if q=="" { return 0, true }; add:=CountContains(strings.ToLower(e.Annotation.Summary), q)*2; if add==0 { return 0, false }; return add, true }
func scoreAll(e schema.Entry, q string) (int, bool) { q=strings.ToLower(strings.TrimSpace(q)); if q=="" { return 0, true }; var buf bytes.Buffer; _ = yaml.NewEncoder(&buf).Encode(e); add:=CountContains(strings.ToLower(buf.String()), q); if add==0 { return 0, false }; return add, true }

func CountContains(text, q string) int { if q=="" { return 0 }; terms := strings.Fields(q); score := 0; for _, t := range terms { if t=="" { continue }; idx:=0; for { i := strings.Index(text[idx:], t); if i<0 { break }; score++; idx += i + len(t) } }; return score }
func trimQuotes(s string) string {
    s = strings.TrimSpace(s)
    if len(s) >= 2 {
        if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
            return s[1 : len(s)-1]
        }
    }
    return s
}

func renderTable(w io.Writer, headers []string, rows [][]string) { widths:=computeColWidths(headers, rows); writeColumns(w, headers, widths); writeSeparator(w, widths); writeRows(w, rows, widths) }
func computeColWidths(headers []string, rows [][]string) []int { widths:=make([]int,len(headers)); for i,h := range headers { widths[i]=len(h) }; for _, r := range rows { for i := range headers { if i < len(r) { if l := len(r[i]); l > widths[i] { widths[i] = l } } } }; return widths }
func writeSeparator(w io.Writer, widths []int) { cols:=make([]string,len(widths)); for i,width := range widths { cols[i]=strings.Repeat("-", width) }; writeColumns(w, cols, widths) }
func writeRows(w io.Writer, rows [][]string, widths []int) { for _, r := range rows { writeColumns(w, r, widths) } }
func writeColumns(w io.Writer, cols []string, widths []int) { for i,width:= range widths { val := ""; if i < len(cols) { val = cols[i] }; _, _ = fmt.Fprintf(w, "%-*s", width, val); if i != len(widths)-1 { _, _ = fmt.Fprint(w, "  ") } }; _, _ = fmt.Fprint(w, "\n") }
