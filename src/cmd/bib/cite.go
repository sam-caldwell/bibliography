package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"bibliography/src/internal/names"
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bibliography/src/internal/stringsx"
)

// newCiteCmd creates the "cite" command to print formatted APA7 and in‑text citations for an id.
func newCiteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cite <id>",
		Short: "Print APA7 citation and in-text citation for a work",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			entries, err := store.ReadAll()
			if err != nil {
				return err
			}
			var found *schema.Entry
			for i := range entries {
				if strings.EqualFold(entries[i].ID, id) {
					found = &entries[i]
					break
				}
			}
			if found == nil {
				return fmt.Errorf("no citation found for id %s", id)
			}
			citation := toAPACitation(*found)
			inline := toInTextCitation(*found)
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "\ncitation:\n%s\n\nin text:\n%s\n\n", citation, inline)
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

// toAPACitation builds an APA7 reference string for the given entry.
func toAPACitation(e schema.Entry) string {
	authors := formatAuthors(e.APA7.Authors)
	year := apaYear(e)
	title := strings.TrimSpace(e.APA7.Title)
	cont := strings.TrimSpace(stringsx.FirstNonEmpty(e.APA7.Journal, e.APA7.ContainerTitle))
	vol := strings.TrimSpace(e.APA7.Volume)
	iss := strings.TrimSpace(e.APA7.Issue)
	pgs := strings.TrimSpace(e.APA7.Pages)
	pub := strings.TrimSpace(e.APA7.Publisher)
	doi := strings.TrimSpace(e.APA7.DOI)
	url := strings.TrimSpace(e.APA7.URL)

	var b strings.Builder
	if authors != "" {
		b.WriteString(authors)
		b.WriteString(" ")
	}
	if year != "" {
		b.WriteString("(")
		b.WriteString(year)
		b.WriteString("). ")
	}
	if title != "" {
		b.WriteString(title)
		b.WriteString(". ")
	}

	b.WriteString(typeDetails(strings.ToLower(e.Type), cont, vol, iss, pgs, pub))

	if doi != "" {
		b.WriteString("https://doi.org/")
		b.WriteString(doi)
		b.WriteString(". ")
	} else if url != "" {
		b.WriteString(url)
		b.WriteString(". ")
	}
	out := strings.TrimSpace(b.String())
	if !strings.HasSuffix(out, ".") {
		out += "."
	}
	return out
}

// typeDetails returns the detail segment for a given entry type.
func typeDetails(typ, cont, vol, iss, pgs, pub string) string {
	// Table-driven dispatch keeps branching out of this function
	if f, ok := detailFormatters[typ]; ok {
		return joinDetails(f(cont, vol, iss, pgs, pub))
	}
	return joinDetails(detailsDefault(cont, vol, iss, pgs, pub))
}

// detailFormatters maps types to their specific detail builders.
var detailFormatters = map[string]func(cont, vol, iss, pgs, pub string) []string{
	"article": detailsArticle,
	"book":    detailsBook,
	"website": detailsWebsite,
	"movie":   detailsMovie,
	"video":   detailsVideo,
	"song":    detailsSong,
	"patent":  detailsPatent,
	"rfc":     detailsRFC,
}

// detailsArticle returns details for an article: container, volume/issue, and pages.
func detailsArticle(cont, vol, iss, pgs, _ string) []string {
	var parts []string
	add(&parts, cont)
	add(&parts, volIssue(vol, iss))
	add(&parts, pgs)
	return parts
}

// detailsBook returns details for a book: publisher.
func detailsBook(_, _, _, _, pub string) []string     { return compact(pub) }
// detailsWebsite returns details for a website: container/site.
func detailsWebsite(cont, _, _, _, _ string) []string { return compact(cont) }
// detailsMovie returns details for a movie: marker and studio/publisher.
func detailsMovie(_, _, _, _, pub string) []string    { return compact("[Film]", pub) }
// detailsVideo returns details for a video: marker and platform name.
func detailsVideo(cont, _, _, _, _ string) []string {
	if strings.TrimSpace(cont) == "" {
		cont = "YouTube"
	}
	return compact("[Video]", cont)
}
// detailsSong returns details for a song: marker and album/label.
func detailsSong(cont, _, _, _, pub string) []string { return compact("[Song]", cont, pub) }
// detailsPatent returns details for a patent: office and publisher.
func detailsPatent(cont, _, _, _, pub string) []string {
	if strings.TrimSpace(cont) != "" {
		cont = "Patent office: " + cont
	}
	return compact(cont, pub)
}
// detailsRFC returns details for an RFC: RFC tag container.
func detailsRFC(cont, _, _, _, _ string) []string {
	if strings.TrimSpace(cont) == "" {
		cont = "RFC"
	}
	return compact(cont)
}
// detailsDefault returns generic details for unrecognized types.
func detailsDefault(cont, _, _, _, pub string) []string { return compact(cont, pub) }

// Helpers
// volIssue formats volume and optional issue as "vol(issue)".
func volIssue(vol, iss string) string {
	vol = strings.TrimSpace(vol)
	iss = strings.TrimSpace(iss)
	if vol == "" {
		return ""
	}
	if iss == "" {
		return vol
	}
	return fmt.Sprintf("%s(%s)", vol, iss)
}
// add appends a trimmed non-empty string to parts.
func add(parts *[]string, s string) {
	if strings.TrimSpace(s) != "" {
		*parts = append(*parts, strings.TrimSpace(s))
	}
}
// compact returns a slice of trimmed non-empty values in order.
func compact(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}
// joinDetails joins detail parts with punctuation and a trailing period.
func joinDetails(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ". ") + ". "
}

// toInTextCitation computes an APA7 in‑text citation for the entry, e.g., (Doe, 2020).
func toInTextCitation(e schema.Entry) string {
	year := apaYear(e)
	// first author family or org
	if len(e.APA7.Authors) == 0 {
		// fallback to publisher or container
		name := strings.TrimSpace(stringsx.FirstNonEmpty(e.APA7.Publisher, e.APA7.ContainerTitle, e.APA7.Journal, e.APA7.Title))
		if name == "" {
			name = "Anon"
		}
		if year == "" {
			year = "n.d."
		}
		return fmt.Sprintf("(%s, %s)", name, year)
	}
	fams := make([]string, 0, len(e.APA7.Authors))
	for _, a := range e.APA7.Authors {
		f := strings.TrimSpace(a.Family)
		if f != "" {
			fams = append(fams, f)
		}
	}
	if len(fams) == 0 {
		fams = []string{"Anon"}
	}
	if year == "" {
		year = "n.d."
	}
	if len(fams) == 1 {
		return fmt.Sprintf("(%s, %s)", fams[0], year)
	}
	if len(fams) == 2 {
		return fmt.Sprintf("(%s & %s, %s)", fams[0], fams[1], year)
	}
	return fmt.Sprintf("(%s et al., %s)", fams[0], year)
}

// apaYear returns the four-digit year for an entry if available.
func apaYear(e schema.Entry) string {
	if e.APA7.Year != nil && *e.APA7.Year > 0 {
		return fmt.Sprintf("%d", *e.APA7.Year)
	}
	d := strings.TrimSpace(e.APA7.Date)
	if len(d) >= 4 {
		return d[:4]
	}
	return ""
}

// formatAuthors renders the authors list in APA style.
func formatAuthors(authors schema.Authors) string {
	if len(authors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(authors))
	for _, a := range authors {
		if s := formatAuthor(a); s != "" {
			parts = append(parts, s)
		}
	}
	return joinOxfordAmp(parts)
}

// formatAuthor renders one author as "Family, G. G.". If family is empty but given is
// present (e.g., corporate/organization captured as Given), it returns the given text.
func formatAuthor(a schema.Author) string {
	fam := strings.TrimSpace(a.Family)
	giv := strings.TrimSpace(a.Given)
	if fam == "" {
		return giv
	}
	if gi := names.Initials(giv); gi != "" {
		return fmt.Sprintf("%s, %s", fam, gi)
	}
	return fam
}

// joinOxfordAmp joins names with an Oxford comma and & before the last: A., B., & C.
func joinOxfordAmp(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + ", & " + parts[1]
	default:
		return strings.Join(parts[:len(parts)-1], ", ") + ", & " + parts[len(parts)-1]
	}
}

// firstNonEmpty removed; using stringsx.FirstNonEmpty
