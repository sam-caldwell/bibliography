package verifycmd

import (
    "fmt"
    "strings"
    "time"
    "net/http"

    "github.com/spf13/cobra"

    "bibliography/src/internal/store"
    "bibliography/src/internal/schema"
    "bibliography/src/internal/doi"
    booksearch "bibliography/src/internal/booksearch"
    webfetch "bibliography/src/internal/webfetch"
    youtube "bibliography/src/internal/video"
    rfcpkg "bibliography/src/internal/rfc"
)

// New returns the verify command which marks a record as verified.
func New() *cobra.Command {
    var id string
    var by string
    var listPending bool
    var showID bool
    var auto bool
    cmd := &cobra.Command{
        Use:   "verify",
        Short: "Mark a citation as verified (sets verified=true, updates modified/verified_by)",
        RunE: func(cmd *cobra.Command, args []string) error {
            if auto {
                return runAuto(cmd)
            }
            if listPending {
                es, err := store.ListUnverified()
                if err != nil { return err }
                if showID {
                    for _, e := range es { _, _ = fmt.Fprintln(cmd.OutOrStdout(), e.ID) }
                    return nil
                }
                renderTable(cmd, es)
                return nil
            }
            if strings.TrimSpace(id) == "" {
                return fmt.Errorf("--id is required")
            }
            who := strings.TrimSpace(by)
            if who == "" { who = store.GetGitUserName() }
            if err := store.VerifyByID(id, who); err != nil {
                return err
            }
            _, err := fmt.Fprintf(cmd.OutOrStdout(), "verified %s by %s\n", id, who)
            return err
        },
    }
    cmd.Flags().StringVar(&id, "id", "", "Entry ID (uuid)")
    cmd.Flags().StringVar(&by, "by", "", "Verifier name (defaults to git user.name)")
    cmd.Flags().BoolVar(&listPending, "list-pending", false, "List entries where verified=false")
    cmd.Flags().BoolVar(&showID, "showId", false, "With --list-pending, print only IDs")
    cmd.Flags().BoolVar(&auto, "auto", false, "Attempt to auto-verify unverified entries with provider consensus")
    return cmd
}

func renderTable(cmd *cobra.Command, es []schema.Entry) {
    headers := []string{"id", "type", "title", "author"}
    rows := make([][]string, 0, len(es))
    for _, e := range es {
        rows = append(rows, []string{e.ID, e.Type, e.APA7.Title, firstAuthor(e)})
    }
    widths := computeColWidths(headers, rows)
    writeColumns(cmd, headers, widths)
    writeSeparator(cmd, widths)
    for _, r := range rows { writeColumns(cmd, r, widths) }
}

func firstAuthor(e schema.Entry) string {
    if len(e.APA7.Authors) == 0 { return "" }
    a := e.APA7.Authors[0]
    fam := strings.TrimSpace(a.Family)
    giv := strings.TrimSpace(a.Given)
    if fam == "" { return giv }
    if giv == "" { return fam }
    return fam + ", " + giv
}

func computeColWidths(headers []string, rows [][]string) []int {
    widths := make([]int, len(headers))
    for i, h := range headers { widths[i] = len(h) }
    for _, r := range rows {
        for i := range headers {
            if i < len(r) {
                if l := len(r[i]); l > widths[i] { widths[i] = l }
            }
        }
    }
    return widths
}

func writeSeparator(cmd *cobra.Command, widths []int) {
    cols := make([]string, len(widths))
    for i, w := range widths { cols[i] = strings.Repeat("-", w) }
    writeColumns(cmd, cols, widths)
}

func writeColumns(cmd *cobra.Command, cols []string, widths []int) {
    for i, w := range widths {
        val := ""
        if i < len(cols) { val = cols[i] }
        fmt.Fprintf(cmd.OutOrStdout(), "%-*s", w, val)
        if i != len(widths)-1 { fmt.Fprint(cmd.OutOrStdout(), "  ") }
    }
    fmt.Fprint(cmd.OutOrStdout(), "\n")
}

// --- Auto verification ---

func runAuto(cmd *cobra.Command) error {
    es, err := store.ListUnverified()
    if err != nil { return err }
    if len(es) == 0 {
        _, _ = fmt.Fprintln(cmd.OutOrStdout(), "no unverified entries")
        return nil
    }
    total := len(es)
    eligible := 0
    verifiedCount := 0
    for _, e := range es {
        provs, ok := verifyWithProviders(cmd, e)
        if !ok || len(provs) < 2 {
            continue
        }
        eligible++
        // Present proposed record (current entry) and prompt
        fmt.Fprintf(cmd.OutOrStdout(), "Proposed verification for %s (%s)\n", e.ID, e.APA7.Title)
        fmt.Fprintln(cmd.OutOrStdout(), entryToYAML(e))
        fmt.Fprint(cmd.OutOrStdout(), "verified (y/n)? ")
        var resp string
        fmt.Fscan(cmd.InOrStdin(), &resp)
        if strings.ToLower(strings.TrimSpace(resp)) == "y" {
            // Update source to first provider and mark verified
            _ = store.UpdateSourceByID(e.ID, provs[0])
            who := store.GetGitUserName()
            if err := store.VerifyByID(e.ID, who); err != nil { return err }
            fmt.Fprintf(cmd.OutOrStdout(), "verified %s by %s (source=%s)\n", e.ID, who, provs[0])
            verifiedCount++
        }
    }
    fmt.Fprintf(cmd.OutOrStdout(), "auto-verify summary: %d verified, %d eligible, %d total unverified\n", verifiedCount, eligible, total)
    return nil
}

// verifyWithProviders attempts provider checks based on entry type and available identifiers.
// Returns a slice of provider labels that succeeded. If two or more succeed, the entry is eligible.
func verifyWithProviders(cmd *cobra.Command, e schema.Entry) ([]string, bool) {
    var providers []string
    // Article: try DOI and URL fetch
    if strings.EqualFold(e.Type, "article") {
        if strings.TrimSpace(e.APA7.DOI) != "" {
            if _, err := doi.FetchArticleByDOI(cmd.Context(), e.APA7.DOI); err == nil {
                providers = append(providers, "doi.org")
            }
        }
        if strings.TrimSpace(e.APA7.URL) != "" {
            if _, err := webfetch.FetchArticleByURL(cmd.Context(), e.APA7.URL); err == nil {
                providers = append(providers, "web")
            }
        }
        return providers, len(providers) >= 2
    }
    // Book: try ISBN and Title/Author lookup via booksearch
    if strings.EqualFold(e.Type, "book") {
        if strings.TrimSpace(e.APA7.ISBN) != "" {
            if _, provider, _, err := booksearch.LookupBookByISBN(cmd.Context(), e.APA7.ISBN); err == nil && provider != "" {
                providers = append(providers, provider)
            }
        }
        // build a minimal author hint (family name of first author if present)
        auth := ""
        if len(e.APA7.Authors) > 0 { auth = e.APA7.Authors[0].Family }
        if _, provider, _, err := booksearch.LookupBookByTitleAuthor(cmd.Context(), e.APA7.Title, auth); err == nil && provider != "" {
            // avoid duplicate same provider label
            dup := false
            for _, p := range providers { if p == provider { dup = true; break } }
            if !dup { providers = append(providers, provider) }
        }
        return providers, len(providers) >= 2
    }
    // Website: consider URL accessibility and webfetch
    if strings.EqualFold(e.Type, "website") && strings.TrimSpace(e.APA7.URL) != "" {
        ok := urlAccessible(cmd.Context(), e.APA7.URL)
        if ok { providers = append(providers, "head/get") }
        if _, err := webfetch.FetchArticleByURL(cmd.Context(), e.APA7.URL); err == nil {
            providers = append(providers, "web")
        }
        return providers, len(providers) >= 2
    }
    // Video: YouTube provider only
    if strings.EqualFold(e.Type, "video") && strings.TrimSpace(e.APA7.URL) != "" {
        if _, err := youtube.FetchYouTube(cmd.Context(), e.APA7.URL); err == nil {
            providers = append(providers, "youtube")
        }
        return providers, len(providers) >= 2
    }
    // RFC: rfc-editor
    if strings.EqualFold(e.Type, "rfc") && strings.TrimSpace(e.APA7.Title) != "" {
        // Try to infer RFC number from title like "RFC 5424" (basic)
        prov := ""
        if _, err := rfcpkg.FetchRFC(cmd.Context(), e.APA7.Title); err == nil { prov = "rfc-editor" }
        if prov != "" { providers = append(providers, prov) }
        return providers, len(providers) >= 2
    }
    // Movie, Song, Patent: insufficient independent providers in this CLI to form 2/3 consensus reliably
    return providers, false
}

func urlAccessible(ctx interface{ Done() <-chan struct{} }, u string) bool {
    c := &http.Client{Timeout: 10 * time.Second}
    if req, err := http.NewRequest(http.MethodHead, u, nil); err == nil {
        if resp, err := c.Do(req); err == nil {
            resp.Body.Close()
            if resp.StatusCode >= 200 && resp.StatusCode < 400 { return true }
        }
    }
    if req, err := http.NewRequest(http.MethodGet, u, nil); err == nil {
        if resp, err := c.Do(req); err == nil {
            resp.Body.Close()
            if resp.StatusCode >= 200 && resp.StatusCode < 400 { return true }
        }
    }
    return false
}
