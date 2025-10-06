package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"bibliography/src/internal/doi"
	"bibliography/src/internal/gitutil"
	moviefetch "bibliography/src/internal/movie"
	"bibliography/src/internal/openlibrary"
	rfcpkg "bibliography/src/internal/rfc"
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bibliography/src/internal/summarize"
	youtube "bibliography/src/internal/video"
	webfetch "bibliography/src/internal/webfetch"
)

// indirections for testability
var (
	commitAndPush = gitutil.CommitAndPush
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add annotated citations via OpenLibrary/DOI (no OpenAI)",
	}

	// add site <url>
	var siteKeywords string
	site := &cobra.Command{
		Use:   "site [url]",
		Short: "Add a website by URL or prompt for manual entry",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) >= 1 && strings.TrimSpace(args[0]) != "" {
				url := args[0]
				return doAddWithKeywords(cmd.Context(), "website", map[string]string{"url": url}, parseKeywordsCSV(siteKeywords))
			}
			// Manual entry when no URL provided
			return manualAdd(cmd, "website", parseKeywordsCSV(siteKeywords))
		},
	}
	site.Flags().StringVar(&siteKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add book [--name ...] [--author ...] [--isbn ...]
	var bookName, bookAuthor, bookISBN, bookKeywords string
	book := &cobra.Command{
		Use:   "book",
		Short: "Add a book (flags or manual entry)",
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
			// If ISBN provided, use OpenLibrary instead of manual
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
			if bookName == "" && bookAuthor == "" {
				return manualAdd(cmd, "book", parseKeywordsCSV(bookKeywords))
			}
			return doAddWithKeywords(cmd.Context(), "book", hints, parseKeywordsCSV(bookKeywords))
		},
	}
	book.Flags().StringVar(&bookName, "name", "", "Book title")
	book.Flags().StringVar(&bookAuthor, "author", "", "Author (Family, Given)")
	book.Flags().StringVar(&bookISBN, "isbn", "", "ISBN")
	book.Flags().StringVar(&bookKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add movie <name> [--date YYYY-MM-DD]
	var movieDate, movieKeywords string
	movie := &cobra.Command{
		Use:   "movie [name]",
		Short: "Add a movie (name or manual entry)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				title := strings.Join(args, " ")
				// Try Google Knowledge Graph first
				e, err := moviefetch.FetchMovie(cmd.Context(), title, movieDate)
				if err != nil {
					// Fallback to OpenAI if available
					if me, merr := summarize.GenerateMovieFromTitleAndDate(cmd.Context(), title, movieDate); merr == nil {
						e = me
					} else {
						// If both providers fail, fall back to minimal construction
						hints := map[string]string{"title": title}
						if movieDate != "" {
							hints["date"] = movieDate
						}
						return doAddWithKeywords(cmd.Context(), "movie", hints, parseKeywordsCSV(movieKeywords))
					}
				}
				if ks := parseKeywordsCSV(movieKeywords); len(ks) > 0 {
					e.Annotation.Keywords = ks
				}
				if len(e.Annotation.Keywords) == 0 {
					e.Annotation.Keywords = []string{"movie"}
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
			return manualAdd(cmd, "movie", parseKeywordsCSV(movieKeywords))
		},
	}
	movie.Flags().StringVar(&movieDate, "date", "", "release date YYYY-MM-DD")
	movie.Flags().StringVar(&movieKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add article [--doi ...] [--title ...] [--author ...] [--journal ...] [--date ...]
	var artDOI, artURL, artTitle, artAuthor, artJournal, artDate, artKeywords string
	article := &cobra.Command{
		Use:   "article",
		Short: "Add a journal or magazine article (flags or manual entry)",
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
			} else if strings.TrimSpace(artURL) != "" {
				e, err := webfetch.FetchArticleByURL(cmd.Context(), artURL)
				if err != nil {
					// Fallback when access is denied: 401/403 -> try OpenAI citation
					if hs, ok := err.(*webfetch.HTTPStatusError); ok && (hs.Status == 401 || hs.Status == 403) {
						if ce, cerr := summarize.GenerateCitationFromURL(cmd.Context(), artURL); cerr == nil {
							e = ce
						} else {
							return fmt.Errorf("access denied (%d) and OpenAI fallback failed: %v", hs.Status, cerr)
						}
					} else {
						return err
					}
				}
				if ks := parseKeywordsCSV(artKeywords); len(ks) > 0 {
					e.Annotation.Keywords = ks
				}
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
			// Manual entry fallback when no DOI/URL provided and minimal hints empty
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
			if len(hints) == 0 {
				return manualAdd(cmd, "article", parseKeywordsCSV(artKeywords))
			}
			return doAddWithKeywords(cmd.Context(), "article", hints, parseKeywordsCSV(artKeywords))
		},
	}
	article.Flags().StringVar(&artDOI, "doi", "", "DOI of the article")
	article.Flags().StringVar(&artURL, "url", "", "URL of an online article to fetch via OpenGraph/JSON-LD")
	article.Flags().StringVar(&artTitle, "title", "", "Article title")
	article.Flags().StringVar(&artAuthor, "author", "", "Author (Family, Given)")
	article.Flags().StringVar(&artJournal, "journal", "", "Journal or publication name")
	article.Flags().StringVar(&artDate, "date", "", "Publication date YYYY-MM-DD")
	article.Flags().StringVar(&artKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// add rfc <rfcNumber>
	var rfcKeywords string
	rfc := &cobra.Command{
		Use:   "rfc [rfcNumber]",
		Short: "Add an RFC by number or prompt for manual entry",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
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
			}
			return manualAdd(cmd, "rfc", parseKeywordsCSV(rfcKeywords))
		},
	}
	rfc.Flags().StringVar(&rfcKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	// --- Video ---
	var ytURL, videoKeywords string
	video := &cobra.Command{
		Use:   "video",
		Short: "Add a video (YouTube URL or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(ytURL) != "" {
				e, err := youtube.FetchYouTube(cmd.Context(), ytURL)
				if err != nil {
					return err
				}
				if ks := parseKeywordsCSV(videoKeywords); len(ks) > 0 {
					e.Annotation.Keywords = ks
				}
				if len(e.Annotation.Keywords) == 0 {
					e.Annotation.Keywords = []string{"video"}
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
			return manualAdd(cmd, "video", parseKeywordsCSV(videoKeywords))
		},
	}
	video.Flags().StringVar(&ytURL, "youtube", "", "YouTube video URL to fetch via oEmbed")
	video.Flags().StringVar(&videoKeywords, "keywords", "", "comma-delimited keywords to set on the entry")

	cmd.AddCommand(site, book, movie, article, video, rfc)
	return cmd
}

func doAdd(ctx context.Context, typ string, hints map[string]string) error {
	return doAddWithKeywords(ctx, typ, hints, nil)
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

func doAddWithKeywords(ctx context.Context, typ string, hints map[string]string, extraKeywords []string) error {
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
			return fmt.Errorf("title is required for %s adds without external metadata", typ)
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
	e.ID = schema.NewID()
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

// --- Manual entry helpers ---

func manualAdd(cmd *cobra.Command, typ string, extraKeywords []string) error {
	in := cmd.InOrStdin()
	out := cmd.OutOrStdout()
	// Basic prompts
	title := strings.TrimSpace(prompt(cmd, in, out, "Title (required): "))
	if title == "" {
		return fmt.Errorf("title is required")
	}
	authorsIn := strings.TrimSpace(prompt(cmd, in, out, "Authors (semicolon-separated; use 'Family, Given' or organization name): "))
	date := strings.TrimSpace(prompt(cmd, in, out, "Date (YYYY-MM-DD; optional): "))
	var yearPtr *int
	if len(date) >= 4 {
		var y int
		if _, err := fmt.Sscanf(date[:4], "%d", &y); err == nil && y >= 1000 {
			y2 := y
			yearPtr = &y2
		}
	}
	url := strings.TrimSpace(prompt(cmd, in, out, "URL (optional): "))
	doi := ""
	isbn := ""
	journal := ""
	publisher := ""
	switch typ {
	case "article":
		journal = strings.TrimSpace(prompt(cmd, in, out, "Journal/Container (optional): "))
		doi = strings.TrimSpace(prompt(cmd, in, out, "DOI (optional): "))
	case "book":
		publisher = strings.TrimSpace(prompt(cmd, in, out, "Publisher (optional): "))
		isbn = strings.TrimSpace(prompt(cmd, in, out, "ISBN (optional): "))
	case "website":
		// nothing extra
	case "movie":
		// accept publisher as studio
		publisher = strings.TrimSpace(prompt(cmd, in, out, "Studio/Publisher (optional): "))
	case "rfc":
		// Allow manual entry for RFC basics
		publisher = strings.TrimSpace(prompt(cmd, in, out, "Publisher (default IETF; optional): "))
		if publisher == "" {
			publisher = "IETF"
		}
	}
	summary := strings.TrimSpace(prompt(cmd, in, out, "Summary (required): "))
	if summary == "" {
		// Provide a sensible default to satisfy validation
		summary = fmt.Sprintf("Bibliographic record for %s (manually entered).", title)
	}
	keywordsIn := strings.TrimSpace(prompt(cmd, in, out, "Keywords (comma-separated; optional): "))
	keywords := parseKeywordsCSV(keywordsIn)
	if len(keywords) == 0 {
		keywords = []string{typ}
	}
	if len(extraKeywords) > 0 {
		keywords = append(keywords, extraKeywords...)
	}

	// Build entry
	var e schema.Entry
	e.Type = typ
	e.ID = schema.NewID()
	e.APA7.Title = title
	e.APA7.ContainerTitle = journal
	e.APA7.Journal = journal
	e.APA7.Publisher = publisher
	if yearPtr != nil {
		e.APA7.Year = yearPtr
	}
	e.APA7.Date = date
	e.APA7.URL = url
	e.APA7.DOI = doi
	e.APA7.ISBN = isbn
	if strings.TrimSpace(e.APA7.URL) != "" {
		e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	}
	// Authors
	for _, name := range splitAuthorsBySemi(authorsIn) {
		fam, giv := parseAuthor(name)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	e.Annotation.Summary = summary
	e.Annotation.Keywords = keywords

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
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
	return nil
}

func prompt(cmd *cobra.Command, in io.Reader, out io.Writer, q string) string {
	// write prompt
	fmt.Fprint(out, q)
	// read line
	br := bufio.NewReader(in)
	s, _ := br.ReadString('\n')
	return strings.TrimRight(s, "\r\n")
}

func splitAuthorsBySemi(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
