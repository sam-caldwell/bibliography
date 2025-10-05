package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"bibliography/src/internal/doi"
	"bibliography/src/internal/gitutil"
	"bibliography/src/internal/openlibrary"
	rfcpkg "bibliography/src/internal/rfc"
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
)

// indirections for testability
var (
	commitAndPush = gitutil.CommitAndPush
)

func newLookupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add annotated citations via OpenLibrary/DOI (no OpenAI)",
	}

	// add site <url>
	var siteKeywords string
	site := &cobra.Command{
		Use:   "site <url>",
		Short: "Add a website by URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			return doLookupWithKeywords(cmd.Context(), "website", map[string]string{"url": url}, parseKeywordsCSV(siteKeywords))
		},
	}
	site.Flags().StringVar(&siteKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add book [--name ...] [--author ...] [--isbn ...]
	var bookName, bookAuthor, bookISBN, bookKeywords string
	book := &cobra.Command{
		Use:   "book",
		Short: "Add a book",
		RunE: func(cmd *cobra.Command, args []string) error {
			hints := map[string]string{}
			if bookName != "" {
				hints["title"] = bookName
			}
			if bookAuthor != "" {
				hints["author"] = bookAuthor
			}
			if bookISBN != "" {
				hints["isbn"] = bookISBN
			}
			// If ISBN provided, use OpenLibrary instead of OpenAI
			if bookISBN != "" {
				e, err := openlibrary.FetchBookByISBN(cmd.Context(), bookISBN)
				if err != nil {
					return err
				}
				// If keywords flag provided, override
				if ks := parseKeywordsCSV(bookKeywords); len(ks) > 0 {
					e.Annotation.Keywords = ks
				}
				path, err := store.WriteEntry(e)
				if err != nil {
					return err
				}
				if err := commitAndPush([]string{path}, fmt.Sprintf("add citation: %s", e.ID)); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
				return nil
			}
			return doLookupWithKeywords(cmd.Context(), "book", hints, parseKeywordsCSV(bookKeywords))
		},
	}
	book.Flags().StringVar(&bookName, "name", "", "Book title")
	book.Flags().StringVar(&bookAuthor, "author", "", "Author (Family, Given)")
	book.Flags().StringVar(&bookISBN, "isbn", "", "ISBN")
	book.Flags().StringVar(&bookKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add movie <name> [--date YYYY-MM-DD]
	var movieDate, movieKeywords string
	movie := &cobra.Command{
		Use:   "movie <name>",
		Short: "Add a movie",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hints := map[string]string{"title": strings.Join(args, " ")}
			if movieDate != "" {
				hints["date"] = movieDate
			}
			return doLookupWithKeywords(cmd.Context(), "movie", hints, parseKeywordsCSV(movieKeywords))
		},
	}
	movie.Flags().StringVar(&movieDate, "date", "", "release date YYYY-MM-DD")
	movie.Flags().StringVar(&movieKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add article [--doi ...] [--title ...] [--author ...] [--journal ...] [--date ...]
	var artDOI, artTitle, artAuthor, artJournal, artDate, artKeywords string
	article := &cobra.Command{
		Use:   "article",
		Short: "Add a journal or magazine article",
		RunE: func(cmd *cobra.Command, args []string) error {
			hints := map[string]string{}
			if artDOI != "" {
				// Resolve via DOI using doi.org, without OpenAI
				e, err := doi.FetchArticleByDOI(cmd.Context(), artDOI)
				if err != nil {
					return err
				}
				// Ensure DOI field is recorded even if provider response omits it
				if strings.TrimSpace(e.APA7.DOI) == "" {
					e.APA7.DOI = strings.TrimSpace(artDOI)
				}
				if ks := parseKeywordsCSV(artKeywords); len(ks) > 0 {
					e.Annotation.Keywords = ks
				}
				// Ensure at least one keyword
				if len(e.Annotation.Keywords) == 0 {
					e.Annotation.Keywords = []string{"article"}
				}
				path, err := store.WriteEntry(e)
				if err != nil {
					return err
				}
				if err := commitAndPush([]string{path}, fmt.Sprintf("add citation: %s", e.ID)); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
				return nil
			}
			if artTitle != "" {
				hints["title"] = artTitle
			}
			if artAuthor != "" {
				hints["author"] = artAuthor
			}
			if artJournal != "" {
				hints["journal"] = artJournal
			}
			if artDate != "" {
				hints["date"] = artDate
			}
			return doLookupWithKeywords(cmd.Context(), "article", hints, parseKeywordsCSV(artKeywords))
		},
	}
	article.Flags().StringVar(&artDOI, "doi", "", "DOI of the article")
	article.Flags().StringVar(&artTitle, "title", "", "Article title")
	article.Flags().StringVar(&artAuthor, "author", "", "Author (Family, Given)")
	article.Flags().StringVar(&artJournal, "journal", "", "Journal or publication name")
	article.Flags().StringVar(&artDate, "date", "", "Publication date YYYY-MM-DD")
	article.Flags().StringVar(&artKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add rfc <rfcNumber>
	var rfcKeywords string
	rfc := &cobra.Command{
		Use:   "rfc <rfcNumber>",
		Short: "Add an RFC by number (e.g., rfc5424)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := rfcpkg.FetchRFC(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if ks := parseKeywordsCSV(rfcKeywords); len(ks) > 0 {
				e.Annotation.Keywords = ks
			}
			path, err := store.WriteEntry(e)
			if err != nil {
				return err
			}
			if err := commitAndPush([]string{path}, fmt.Sprintf("add citation: %s", e.ID)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}
	rfc.Flags().StringVar(&rfcKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	cmd.AddCommand(site, book, movie, article, rfc)
	return cmd
}

func doLookup(ctx context.Context, typ string, hints map[string]string) error {
	return doLookupWithKeywords(ctx, typ, hints, nil)
}

func parseKeywordsCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[strings.ToLower(p)] {
			continue
		}
		seen[strings.ToLower(p)] = true
		out = append(out, p)
	}
	return out
}

func doLookupWithKeywords(ctx context.Context, typ string, hints map[string]string, extraKeywords []string) error {
	var e schema.Entry
	e.Type = typ
	title := strings.TrimSpace(hints["title"])
	switch typ {
	case "website":
		if title == "" {
			if u := strings.TrimSpace(hints["url"]); u != "" {
				if pu, err := url.Parse(u); err == nil && pu.Host != "" {
					title = pu.Host
				} else {
					title = u
				}
			}
		}
	default:
		if title == "" {
			return fmt.Errorf("title is required for %s lookups without external metadata", typ)
		}
	}
	e.APA7.Title = title
	if v := strings.TrimSpace(hints["journal"]); v != "" {
		e.APA7.Journal = v
		e.APA7.ContainerTitle = v
	}
	if v := strings.TrimSpace(hints["date"]); v != "" {
		e.APA7.Date = v
		if len(v) >= 4 {
			var y int
			_, _ = fmt.Sscanf(v[:4], "%d", &y)
			if y >= 1000 {
				e.APA7.Year = &y
			}
		}
	}
	if v := strings.TrimSpace(hints["url"]); v != "" {
		e.APA7.URL = v
	}
	if v := strings.TrimSpace(hints["author"]); v != "" {
		fam, giv := parseAuthor(v)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	if v := strings.TrimSpace(hints["isbn"]); v != "" {
		e.APA7.ISBN = v
	}
	if v := strings.TrimSpace(hints["doi"]); v != "" {
		e.APA7.DOI = v
		if e.APA7.URL == "" {
			e.APA7.URL = "https://doi.org/" + v
		}
	}
	if e.APA7.URL != "" {
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	}
	e.ID = schema.Slugify(e.APA7.Title, e.APA7.Year)
	if len(extraKeywords) > 0 {
		e.Annotation.Keywords = extraKeywords
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{typ}
	}
	if e.APA7.Title != "" {
		if e.APA7.Journal != "" {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s in %s (manually constructed).", e.APA7.Title, e.APA7.Journal)
		} else {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (manually constructed).", e.APA7.Title)
		}
	}
	if err := e.Validate(); err != nil {
		return err
	}
	path, err := store.WriteEntry(e)
	if err != nil {
		return err
	}
	if err := commitAndPush([]string{path}, fmt.Sprintf("add citation: %s", e.ID)); err != nil {
		return err
	}
	// The command context is not passed in here; printing is for CLI feedback only.
	fmt.Fprintf(os.Stdout, "wrote %s\n", path)
	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func parseAuthor(s string) (family, given string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.Index(s, ","); i >= 0 {
		family = strings.TrimSpace(s[:i])
		given = strings.TrimSpace(s[i+1:])
		return family, given
	}
	return s, ""
}
