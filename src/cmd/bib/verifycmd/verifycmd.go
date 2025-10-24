package verifycmd

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	booksearch "bibliography/src/internal/booksearch"
	"bibliography/src/internal/doi"
	movpkg "bibliography/src/internal/movie"
	rfcpkg "bibliography/src/internal/rfc"
	"bibliography/src/internal/schema"
	songpkg "bibliography/src/internal/song"
	"bibliography/src/internal/store"
	youtube "bibliography/src/internal/video"
	webfetch "bibliography/src/internal/webfetch"
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
				if err != nil {
					return err
				}
				if showID {
					for _, e := range es {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), e.ID)
					}
					return nil
				}
				renderTable(cmd, es)
				return nil
			}
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("--id is required")
			}
			who := strings.TrimSpace(by)
			if who == "" {
				who = store.GetGitUserName()
			}
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
	for _, r := range rows {
		writeColumns(cmd, r, widths)
	}
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

func computeColWidths(headers []string, rows [][]string) []int {
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
	return widths
}

func writeSeparator(cmd *cobra.Command, widths []int) {
	cols := make([]string, len(widths))
	for i, w := range widths {
		cols[i] = strings.Repeat("-", w)
	}
	writeColumns(cmd, cols, widths)
}

func writeColumns(cmd *cobra.Command, cols []string, widths []int) {
	for i, w := range widths {
		val := ""
		if i < len(cols) {
			val = cols[i]
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-*s", w, val)
		if i != len(widths)-1 {
			fmt.Fprint(cmd.OutOrStdout(), "  ")
		}
	}
	fmt.Fprint(cmd.OutOrStdout(), "\n")
}

// --- Auto verification ---

func runAuto(cmd *cobra.Command) error {
	es, err := store.ListUnverified()
	if err != nil {
		return err
	}
	if len(es) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no unverified entries")
		return nil
	}
	total := len(es)
	eligible := 0
	verifiedCount := 0
	for _, e := range es {
		provs, ok := verifyWithProviders(cmd, e)
		if !ok {
			continue
		}
		eligible++
		// Present proposed record (current entry) and prompt
		fmt.Fprintf(cmd.OutOrStdout(), "Proposed verification for %s (%s)\n", e.ID, e.APA7.Title)
		if len(provs) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "providers: %s\n", strings.Join(provs, ", "))
		}
		fmt.Fprintln(cmd.OutOrStdout(), entryToYAML(e))
		fmt.Fprint(cmd.OutOrStdout(), "verified (y/n)? ")
		var resp string
		fmt.Fscan(cmd.InOrStdin(), &resp)
		if strings.ToLower(strings.TrimSpace(resp)) == "y" {
			// Update source to first provider and mark verified
			_ = store.UpdateSourceByID(e.ID, provs[0])
			who := store.GetGitUserName()
			if err := store.VerifyByID(e.ID, who); err != nil {
				return err
			}
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
	// Article: try DOI and URL fetch; when URL present, consider both structured fetch and HEAD/GET accessibility
	if strings.EqualFold(e.Type, "article") {
		doiOK := false
		if strings.TrimSpace(e.APA7.DOI) != "" {
			if _, err := doi.FetchArticleByDOI(cmd.Context(), e.APA7.DOI); err == nil {
				providers = append(providers, "doi.org")
				doiOK = true
			}
		}
		if strings.TrimSpace(e.APA7.URL) != "" {
			if _, err := webfetch.FetchArticleByURL(cmd.Context(), e.APA7.URL); err == nil {
				providers = append(providers, "web")
			}
			if urlAccessible(cmd.Context(), e.APA7.URL) {
				providers = append(providers, "head/get")
			}
			if hasHTMLTitle(e.APA7.URL) {
				// Treat presence of HTML <title> as a weak independent signal
				providers = append(providers, "html")
			}
		}
		// Accept DOI-only for articles if DOI resolves; otherwise require 2 signals.
		if doiOK {
			return providers, true
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
		if len(e.APA7.Authors) > 0 {
			auth = e.APA7.Authors[0].Family
		}
		if _, provider, _, err := booksearch.LookupBookByTitleAuthor(cmd.Context(), e.APA7.Title, auth); err == nil && provider != "" {
			// avoid duplicate same provider label
			dup := false
			for _, p := range providers {
				if p == provider {
					dup = true
					break
				}
			}
			if !dup {
				providers = append(providers, provider)
			}
		}
		return providers, len(providers) >= 2
	}
	// Website: consider structured fetch and URL accessibility (HTTP 200 is sufficient)
	if strings.EqualFold(e.Type, "website") && strings.TrimSpace(e.APA7.URL) != "" {
		if _, err := webfetch.FetchArticleByURL(cmd.Context(), e.APA7.URL); err == nil {
			providers = append(providers, "web")
		}
		if urlAccessible(cmd.Context(), e.APA7.URL) {
			providers = append(providers, "head/get")
		}
		return providers, len(providers) >= 1
	}
	// Movie: OMDb/TMDb (1 provider sufficient)
	if strings.EqualFold(e.Type, "movie") && strings.TrimSpace(e.APA7.Title) != "" {
		if _, prov, err := movpkg.FetchMovieWithProvider(cmd.Context(), e.APA7.Title, e.APA7.Date); err == nil && prov != "" {
			providers = append(providers, prov)
		}
		return providers, len(providers) >= 1
	}
	// Song: iTunes/MusicBrainz (1 provider sufficient)
	if strings.EqualFold(e.Type, "song") && strings.TrimSpace(e.APA7.Title) != "" {
		artist := ""
		if len(e.APA7.Authors) > 0 {
			artist = e.APA7.Authors[0].Family
		}
		if _, prov, err := songpkg.FetchSongWithProvider(cmd.Context(), e.APA7.Title, artist, e.APA7.Date); err == nil && prov != "" {
			providers = append(providers, prov)
		}
		return providers, len(providers) >= 1
	}
	// Video: YouTube provider (1 provider sufficient)
	if strings.EqualFold(e.Type, "video") && strings.TrimSpace(e.APA7.URL) != "" {
		if _, err := youtube.FetchYouTube(cmd.Context(), e.APA7.URL); err == nil {
			providers = append(providers, "youtube")
		}
		return providers, len(providers) >= 1
	}
	// RFC: rfc-editor
	if strings.EqualFold(e.Type, "rfc") && strings.TrimSpace(e.APA7.Title) != "" {
		// Try to infer RFC number from title like "RFC 5424" (basic)
		prov := ""
		if _, err := rfcpkg.FetchRFC(cmd.Context(), e.APA7.Title); err == nil {
			prov = "rfc-editor"
		}
		if prov != "" {
			providers = append(providers, prov)
		}
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
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
		}
	}
	if req, err := http.NewRequest(http.MethodGet, u, nil); err == nil {
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return true
			}
		}
	}
	return false
}

var reHTMLTitle = regexp.MustCompile(`(?is)<title[^>]*>[^<]+</title>`)

// hasHTMLTitle performs a simple GET and checks for a <title> tag.
func hasHTMLTitle(u string) bool {
	c := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return reHTMLTitle.Match(b)
}

// entryToYAML renders a schema.Entry in a human-friendly YAML-like format for preview only.
func entryToYAML(e schema.Entry) string {
	b := &strings.Builder{}
	w := func(indent int, line string) {
		b.WriteString(strings.Repeat(" ", indent))
		b.WriteString(line)
		b.WriteString("\n")
	}
	q := func(s string) string {
		s = strings.ReplaceAll(s, "\"", "\\\"")
		return "\"" + s + "\""
	}
	w(0, "id: "+e.ID)
	w(0, "type: "+e.Type)
	w(0, "apa7:")
	if e.APA7.Title != "" {
		w(2, "title: "+q(e.APA7.Title))
	}
	if e.APA7.ContainerTitle != "" {
		w(2, "container_title: "+q(e.APA7.ContainerTitle))
	}
	if e.APA7.Journal != "" {
		w(2, "journal: "+q(e.APA7.Journal))
	}
	if e.APA7.Publisher != "" {
		w(2, "publisher: "+q(e.APA7.Publisher))
	}
	if e.APA7.PublisherLocation != "" {
		w(2, "publisher_location: "+q(e.APA7.PublisherLocation))
	}
	if e.APA7.Edition != "" {
		w(2, "edition: "+q(e.APA7.Edition))
	}
	if e.APA7.Volume != "" {
		w(2, "volume: "+q(e.APA7.Volume))
	}
	if e.APA7.Issue != "" {
		w(2, "issue: "+q(e.APA7.Issue))
	}
	if e.APA7.Pages != "" {
		w(2, "pages: "+q(e.APA7.Pages))
	}
	if e.APA7.Year != nil {
		w(2, fmt.Sprintf("year: %d", *e.APA7.Year))
	}
	if e.APA7.Date != "" {
		w(2, "date: "+q(e.APA7.Date))
	}
	if e.APA7.DOI != "" {
		w(2, "doi: "+q(e.APA7.DOI))
	}
	if e.APA7.ISBN != "" {
		w(2, "isbn: "+q(e.APA7.ISBN))
	}
	if e.APA7.URL != "" {
		w(2, "url: "+q(e.APA7.URL))
	}
	if e.APA7.Accessed != "" {
		w(2, "accessed: "+q(e.APA7.Accessed))
	}
	if len(e.APA7.Authors) > 0 {
		w(2, "authors:")
		for _, a := range e.APA7.Authors {
			if strings.TrimSpace(a.Family) == "" && strings.TrimSpace(a.Given) == "" {
				continue
			}
			w(4, "- family: "+q(a.Family))
			if strings.TrimSpace(a.Given) != "" {
				w(6, "given: "+q(a.Given))
			}
		}
	}
	w(0, "annotation:")
	if e.Annotation.Summary != "" {
		w(2, "summary: "+q(e.Annotation.Summary))
	}
	if len(e.Annotation.Keywords) > 0 {
		// Render keywords inline list
		items := make([]string, 0, len(e.Annotation.Keywords))
		for _, k := range e.Annotation.Keywords {
			items = append(items, q(k))
		}
		w(2, "keywords: ["+strings.Join(items, ", ")+"]")
	}
	return b.String()
}
