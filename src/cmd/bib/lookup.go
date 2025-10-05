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
	site := &cobra.Command{
		Use:   "site <url>",
		Short: "Lookup a website by URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			return doLookup(cmd.Context(), model, "website", map[string]string{"url": url})
		},
	}

	// lookup book [--name ...] [--author ...] [--isbn ...]
	var bookName, bookAuthor, bookISBN string
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
			return doLookup(cmd.Context(), model, "book", hints)
		},
	}
	book.Flags().StringVar(&bookName, "name", "", "Book title")
	book.Flags().StringVar(&bookAuthor, "author", "", "Author (Family, Given)")
	book.Flags().StringVar(&bookISBN, "isbn", "", "ISBN")

	// lookup movie <name> [--date YYYY-MM-DD]
	var movieDate string
	movie := &cobra.Command{
		Use:   "movie <name>",
		Short: "Lookup a movie",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hints := map[string]string{"title": strings.Join(args, " ")}
			if movieDate != "" {
				hints["date"] = movieDate
			}
			return doLookup(cmd.Context(), model, "movie", hints)
		},
	}
	movie.Flags().StringVar(&movieDate, "date", "", "release date YYYY-MM-DD")

	cmd.AddCommand(site, book, movie)
	return cmd
}

func doLookup(ctx context.Context, model string, typ string, hints map[string]string) error {
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
	// If URL present and accessed missing, set accessed=today
	if e.APA7.URL != "" && e.APA7.Accessed == "" {
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
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
