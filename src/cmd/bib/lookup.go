package main

import (
    "context"
    "fmt"
    "os"
    "strings"
    "time"

	"github.com/spf13/cobra"

	"bibliography/src/internal/ai"
    "bibliography/src/internal/gitutil"
    "bibliography/src/internal/doi"
    "bibliography/src/internal/openlibrary"
    "bibliography/src/internal/schema"
    "bibliography/src/internal/store"
)

// indirections for testability
var (
	newGenerator  = func(model string) (ai.Generator, error) { return ai.NewGeneratorFromEnv(model) }
	commitAndPush = gitutil.CommitAndPush
)

func newLookupCmd() *cobra.Command {
	var model string
	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Create annotated citations via OpenAI",
	}
	cmd.PersistentFlags().StringVar(&model, "model", getEnv("BIBLIOGRAPHY_MODEL", "gpt-4.1-mini"), "OpenAI model")

	// lookup site <url>
	var siteKeywords string
	site := &cobra.Command{
		Use:   "site <url>",
		Short: "Lookup a website by URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			return doLookupWithKeywords(cmd.Context(), model, "website", map[string]string{"url": url}, parseKeywordsCSV(siteKeywords))
		},
	}
	site.Flags().StringVar(&siteKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// lookup book [--name ...] [--author ...] [--isbn ...]
	var bookName, bookAuthor, bookISBN, bookKeywords string
	book := &cobra.Command{
		Use:   "book",
		Short: "Lookup a book",
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
			return doLookupWithKeywords(cmd.Context(), model, "book", hints, parseKeywordsCSV(bookKeywords))
		},
	}
	book.Flags().StringVar(&bookName, "name", "", "Book title")
	book.Flags().StringVar(&bookAuthor, "author", "", "Author (Family, Given)")
	book.Flags().StringVar(&bookISBN, "isbn", "", "ISBN")
	book.Flags().StringVar(&bookKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// lookup movie <name> [--date YYYY-MM-DD]
	var movieDate, movieKeywords string
	movie := &cobra.Command{
		Use:   "movie <name>",
		Short: "Lookup a movie",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hints := map[string]string{"title": strings.Join(args, " ")}
			if movieDate != "" {
				hints["date"] = movieDate
			}
			return doLookupWithKeywords(cmd.Context(), model, "movie", hints, parseKeywordsCSV(movieKeywords))
		},
	}
	movie.Flags().StringVar(&movieDate, "date", "", "release date YYYY-MM-DD")
	movie.Flags().StringVar(&movieKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// lookup article [--doi ...] [--title ...] [--author ...] [--journal ...] [--date ...]
    var artDOI, artTitle, artAuthor, artJournal, artDate, artKeywords string
    article := &cobra.Command{
        Use:   "article",
        Short: "Lookup a journal or magazine article",
        RunE: func(cmd *cobra.Command, args []string) error {
            hints := map[string]string{}
            if artDOI != "" {
                // Resolve via DOI using doi.org, without OpenAI
                e, err := doi.FetchArticleByDOI(cmd.Context(), artDOI)
                if err != nil { return err }
                if ks := parseKeywordsCSV(artKeywords); len(ks) > 0 { e.Annotation.Keywords = ks }
                // Ensure at least one keyword
                if len(e.Annotation.Keywords) == 0 { e.Annotation.Keywords = []string{"article"} }
                path, err := store.WriteEntry(e)
                if err != nil { return err }
                if err := commitAndPush([]string{path}, fmt.Sprintf("add citation: %s", e.ID)); err != nil { return err }
                fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
                return nil
            }
            if artDOI != "" {
                hints["doi"] = artDOI
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
			return doLookupWithKeywords(cmd.Context(), model, "article", hints, parseKeywordsCSV(artKeywords))
		},
	}
	article.Flags().StringVar(&artDOI, "doi", "", "DOI of the article")
	article.Flags().StringVar(&artTitle, "title", "", "Article title")
	article.Flags().StringVar(&artAuthor, "author", "", "Author (Family, Given)")
	article.Flags().StringVar(&artJournal, "journal", "", "Journal or publication name")
	article.Flags().StringVar(&artDate, "date", "", "Publication date YYYY-MM-DD")
	article.Flags().StringVar(&artKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	cmd.AddCommand(site, book, movie, article)
	return cmd
}

func doLookup(ctx context.Context, model string, typ string, hints map[string]string) error {
	return doLookupWithKeywords(ctx, model, typ, hints, nil)
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

func doLookupWithKeywords(ctx context.Context, model string, typ string, hints map[string]string, extraKeywords []string) error {
	// Require API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		return fmt.Errorf("OPENAI_API_KEY not set; please export it to use lookup")
	}
	gen, err := newGenerator(model)
	if err != nil {
		return err
	}
	e, _, err := gen.GenerateYAML(ctx, typ, hints)
	if err != nil {
		return err
	}
	// Ensure ID set if missing
	if strings.TrimSpace(e.ID) == "" {
		e.ID = schema.Slugify(e.APA7.Title, e.APA7.Year)
	}
	// If DOI was provided, prefer doi.org URL when model omitted URL
	if doi := hints["doi"]; doi != "" {
		if e.APA7.URL == "" {
			e.APA7.URL = "https://doi.org/" + strings.TrimSpace(doi)
		}
	}
	// If URL present and accessed missing, set accessed=today
	if e.APA7.URL != "" && e.APA7.Accessed == "" {
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	}
	// If user provided keywords flag, set/override keywords
	if len(extraKeywords) > 0 {
		e.Annotation.Keywords = extraKeywords
	}
	// Fallback: ensure at least one keyword to pass validation
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{typ}
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
