package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"bibliography/src/internal/dates"
	"bibliography/src/internal/doi"
	"bibliography/src/internal/gitutil"
	moviefetch "bibliography/src/internal/movie"
	"bibliography/src/internal/openlibrary"
	rfcpkg "bibliography/src/internal/rfc"
	"bibliography/src/internal/schema"
	songfetch "bibliography/src/internal/song"
	"bibliography/src/internal/store"
	"bibliography/src/internal/summarize"
	youtube "bibliography/src/internal/video"
	"bibliography/src/internal/webfetch"
)

// indirections for testability
var (
	commitAndPush = gitutil.CommitAndPush
)

const (
	msgCommaDelimitedKeywords = "comma-delimited keywords to set on the entry"
	msgWrote                  = "wrote %s\n"
	msgAddCitation            = "add citation: %s"
)

// newAddCmd constructs the root "add" command grouping subcommands for each type.
func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "add", Short: "Add annotated citations via OpenLibrary/DOI (no OpenAI)"}
	cmd.AddCommand(
		buildAddSiteCmd(),
		buildAddBookCmd(),
		buildAddMovieCmd(),
		buildAddSongCmd(),
		buildAddArticleCmd(),
		buildAddVideoCmd(),
		buildAddPatentCmd(),
		buildAddRFCCmd(),
	)
	return cmd
}

// --- Subcommand builders (extracted to reduce complexity) ---

// buildAddSiteCmd creates the "add site" subcommand that adds a website by URL or manual prompts.
func buildAddSiteCmd() *cobra.Command {
	var siteKeywords string
	c := &cobra.Command{
		Use:   "site [url]",
		Short: "Add a website by URL or prompt for manual entry",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) >= 1 && strings.TrimSpace(args[0]) != "" {
				thisUrl := args[0]
				return doAddWithKeywords(cmd.Context(), "website", map[string]string{"url": thisUrl}, parseKeywordsCSV(siteKeywords))
			}
			return manualAdd(cmd, "website", parseKeywordsCSV(siteKeywords))
		},
	}
	c.Flags().StringVar(&siteKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// buildAddBookCmd creates the "add book" subcommand that adds a book by hints, ISBN, or manual prompts.
func buildAddBookCmd() *cobra.Command {
	var bookName, bookAuthor, bookISBN, bookKeywords string
	c := &cobra.Command{
		Use:   "book",
		Short: "Add a book (flags or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Fast-path: ISBN provided → use OpenLibrary/Google Books providers
			if strings.TrimSpace(bookISBN) != "" {
				e, err := openlibrary.FetchBookByISBN(cmd.Context(), bookISBN)
				if err != nil {
					return err
				}
				applyKeywordsOverride(&e, bookKeywords)
				return writeCommitPrint(cmd, e)
			}
			// Manual when no minimal hints
			if strings.TrimSpace(bookName) == "" && strings.TrimSpace(bookAuthor) == "" {
				return manualAdd(cmd, "book", parseKeywordsCSV(bookKeywords))
			}
			// Hinted manual construction path
			hints := hintsBook(bookName, bookAuthor, bookISBN)
			return doAddWithKeywords(cmd.Context(), "book", hints, parseKeywordsCSV(bookKeywords))
		},
	}
	c.Flags().StringVar(&bookName, "name", "", "Book title")
	c.Flags().StringVar(&bookAuthor, "author", "", "Author (Family, Given)")
	c.Flags().StringVar(&bookISBN, "isbn", "", "ISBN")
	c.Flags().StringVar(&bookKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// --- Small helpers to keep RunE concise ---

// applyKeywordsOverride replaces entry keywords from a comma-delimited string when provided.
func applyKeywordsOverride(e *schema.Entry, kwCSV string) {
	if ks := parseKeywordsCSV(kwCSV); len(ks) > 0 {
		e.Annotation.Keywords = ks
	}
}

// writeCommitPrint writes the entry, commits it via git, and prints the written path.
func writeCommitPrint(cmd *cobra.Command, e schema.Entry) error {
	path, err := store.WriteEntry(e)
	if err != nil {
		return err
	}
	if err := commitAndPush([]string{path}, fmt.Sprintf(msgAddCitation, e.ID)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), msgWrote, path)
	return err
}

// hintsBook builds a hints map from optional book flag values.
func hintsBook(name, author, isbn string) map[string]string {
	m := map[string]string{}
	if strings.TrimSpace(name) != "" {
		m["title"] = name
	}
	if strings.TrimSpace(author) != "" {
		m["author"] = author
	}
	if strings.TrimSpace(isbn) != "" {
		m["isbn"] = isbn
	}
	return m
}

// buildAddMovieCmd creates the "add movie" subcommand that adds a film by title/date or manual prompts.
func buildAddMovieCmd() *cobra.Command {
	var movieDate, movieKeywords string
	c := &cobra.Command{
		Use:   "movie [name]",
		Short: "Add a movie (name or manual entry)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				title := strings.Join(args, " ")
				if e, ok := getMovieEntry(cmd.Context(), title, movieDate); ok {
					applyKeywordsOverride(&e, movieKeywords)
					ensureTypeKeyword(&e, "movie")
					return writeCommitPrint(cmd, e)
				}
				return doAddWithKeywords(cmd.Context(), "movie", hintsMovie(title, movieDate), parseKeywordsCSV(movieKeywords))
			}
			return manualAdd(cmd, "movie", parseKeywordsCSV(movieKeywords))
		},
	}
	c.Flags().StringVar(&movieDate, "date", "", "release date YYYY-MM-DD")
	c.Flags().StringVar(&movieKeywords, "keywords", "", "comma-delimited keywords to set on the entry")
	return c
}

// getMovieEntry fetches or generates a movie entry from providers or OpenAI.
func getMovieEntry(ctx context.Context, title, date string) (schema.Entry, bool) {
	if e, err := moviefetch.FetchMovie(ctx, title, date); err == nil {
		return e, true
	}
	if e, err := summarize.GenerateMovieFromTitleAndDate(ctx, title, date); err == nil {
		return e, true
	}
	return schema.Entry{}, false
}

// hintsMovie builds a hints map for a movie using title and optional date.
func hintsMovie(title, date string) map[string]string {
	m := map[string]string{"title": title}
	if strings.TrimSpace(date) != "" {
		m["date"] = date
	}
	return m
}

// ensureTypeKeyword sets a default keyword equal to the entry type when keywords are empty.
func ensureTypeKeyword(e *schema.Entry, typ string) {
	if e == nil {
		return
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{typ}
	}
}

// finalizeAndWrite applies keyword overrides, ensures a default type keyword,
// then writes the entry, commits, and prints the resulting path.
func finalizeAndWrite(cmd *cobra.Command, e schema.Entry, typ string, kwCSV string) error {
	applyKeywordsOverride(&e, kwCSV)
	ensureTypeKeyword(&e, typ)
	return writeCommitPrint(cmd, e)
}

// buildAddSongCmd creates the "add song" subcommand that adds a song by title/artist or manual prompts.
func buildAddSongCmd() *cobra.Command {
	var songArtist, songDate, songKeywords string
	c := &cobra.Command{
		Use:   "song [title]",
		Short: "Add a song (title/artist or manual entry)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				title := strings.Join(args, " ")
				if e, ok := getSongEntry(cmd.Context(), title, songArtist, songDate); ok {
					applyKeywordsOverride(&e, songKeywords)
					ensureTypeKeyword(&e, "song")
					return writeCommitPrint(cmd, e)
				}
				return doAddWithKeywords(cmd.Context(), "song", hintsSong(title, songArtist, songDate), parseKeywordsCSV(songKeywords))
			}
			return manualAdd(cmd, "song", parseKeywordsCSV(songKeywords))
		},
	}
	c.Flags().StringVar(&songArtist, "artist", "", "Artist/performer name")
	c.Flags().StringVar(&songDate, "date", "", "release date YYYY-MM-DD")
	c.Flags().StringVar(&songKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// getSongEntry fetches or generates a song entry from providers or OpenAI.
func getSongEntry(ctx context.Context, title, artist, date string) (schema.Entry, bool) {
	if e, err := songfetch.FetchSong(ctx, title, artist, date); err == nil {
		return e, true
	}
	if e, err := summarize.GenerateSongFromTitleArtistDate(ctx, title, artist, date); err == nil {
		return e, true
	}
	return schema.Entry{}, false
}

// hintsSong builds a hints map for a song using title, optional artist, and optional date.
func hintsSong(title, artist, date string) map[string]string {
	m := map[string]string{"title": title}
	if strings.TrimSpace(artist) != "" {
		m["author"] = artist
	}
	if strings.TrimSpace(date) != "" {
		m["date"] = date
	}
	return m
}

// buildAddArticleCmd creates the "add article" subcommand that adds a journal/web article via DOI/URL or manual.
func buildAddArticleCmd() *cobra.Command {
	var artDOI, artURL, artTitle, artAuthor, artJournal, artDate, artKeywords string
	c := &cobra.Command{
		Use:   "article",
		Short: "Add a journal or magazine article (flags or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if strings.TrimSpace(artDOI) != "" {
				e, err := getArticleByDOI(ctx, artDOI)
				if err != nil {
					return err
				}
				return finalizeAndWrite(cmd, e, "article", artKeywords)
			}
			if strings.TrimSpace(artURL) != "" {
				e, err := getArticleByURL(ctx, artURL)
				if err != nil {
					return err
				}
				return finalizeAndWrite(cmd, e, "article", artKeywords)
			}
			h := hintsArticle(artTitle, artAuthor, artJournal, artDate)
			if len(h) == 0 {
				return manualAdd(cmd, "article", parseKeywordsCSV(artKeywords))
			}
			return doAddWithKeywords(ctx, "article", h, parseKeywordsCSV(artKeywords))
		},
	}
	c.Flags().StringVar(&artDOI, "doi", "", "DOI of the article")
	c.Flags().StringVar(&artURL, "url", "", "URL of an online article to fetch via OpenGraph/JSON-LD")
	c.Flags().StringVar(&artTitle, "title", "", "Article title")
	c.Flags().StringVar(&artAuthor, "author", "", "Author (Family, Given)")
	c.Flags().StringVar(&artJournal, "journal", "", "Journal or publication name")
	c.Flags().StringVar(&artDate, "date", "", "Publication date YYYY-MM-DD")
	c.Flags().StringVar(&artKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// getArticleByDOI fetches an article via DOI, ensuring the DOI field is set.
func getArticleByDOI(ctx context.Context, doiStr string) (schema.Entry, error) {
	e, err := doi.FetchArticleByDOI(ctx, doiStr)
	if err != nil {
		return schema.Entry{}, err
	}
	if strings.TrimSpace(e.APA7.DOI) == "" {
		e.APA7.DOI = strings.TrimSpace(doiStr)
	}
	return e, nil
}

// getArticleByURL fetches an article by URL, falling back to OpenAI when access is denied.
func getArticleByURL(ctx context.Context, u string) (schema.Entry, error) {
	e, err := webfetch.FetchArticleByURL(ctx, u)
	if err == nil {
		return e, nil
	}
	if hs, ok := err.(*webfetch.HTTPStatusError); ok && (hs.Status == 401 || hs.Status == 403) {
		if ce, cerr := summarize.GenerateCitationFromURL(ctx, u); cerr == nil {
			return ce, nil
		} else {
			return schema.Entry{}, fmt.Errorf("access denied (%d) and OpenAI fallback failed: %v", hs.Status, cerr)
		}
	}
	return schema.Entry{}, err
}

// hintsArticle builds a hints map for an article from optional flags.
func hintsArticle(title, author, journal, date string) map[string]string {
	m := map[string]string{}
	if strings.TrimSpace(title) != "" {
		m["title"] = title
	}
	if strings.TrimSpace(author) != "" {
		m["author"] = author
	}
	if strings.TrimSpace(journal) != "" {
		m["journal"] = journal
	}
	if strings.TrimSpace(date) != "" {
		m["date"] = date
	}
	return m
}

// buildAddPatentCmd creates the "add patent" subcommand that adds a patent via URL hints or manual.
func buildAddPatentCmd() *cobra.Command {
	var patURL, patTitle, patInventor, patAssignee, patDate, patKeywords string
	c := &cobra.Command{
		Use:   "patent",
		Short: "Add a patent (flags or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if strings.TrimSpace(patURL) != "" && strings.TrimSpace(patTitle) == "" {
				if e, err := getPatentByURL(cmd.Context(), patURL); err == nil {
					return finalizeAndWrite(cmd, e, "patent", patKeywords)
				} else {
					return err
				}
			}
			h := hintsPatent(patTitle, patInventor, patAssignee, patDate, patURL)
			if len(h) == 0 {
				return manualAdd(cmd, "patent", parseKeywordsCSV(patKeywords))
			}
			return doAddWithKeywords(cmd.Context(), "patent", h, parseKeywordsCSV(patKeywords))
		},
	}
	c.Flags().StringVar(&patURL, "url", "", "URL to a patent page to reference")
	c.Flags().StringVar(&patTitle, "title", "", "Patent title")
	c.Flags().StringVar(&patInventor, "inventor", "", "Inventor (Family, Given)")
	c.Flags().StringVar(&patAssignee, "assignee", "", "Assignee/Office (e.g., USPTO)")
	c.Flags().StringVar(&patDate, "date", "", "Publication date YYYY-MM-DD")
	c.Flags().StringVar(&patKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// getPatentByURL fetches a web page and coerces the resulting entry to type "patent".
func getPatentByURL(ctx context.Context, u string) (schema.Entry, error) {
	e, err := webfetch.FetchArticleByURL(ctx, u)
	if err != nil {
		return schema.Entry{}, err
	}
	e.Type = "patent"
	return e, nil
}

// hintsPatent builds a hints map for a patent from optional fields.
func hintsPatent(title, inventor, assignee, date, url string) map[string]string {
	m := map[string]string{}
	if strings.TrimSpace(title) != "" {
		m["title"] = title
	}
	if strings.TrimSpace(inventor) != "" {
		m["author"] = inventor
	}
	if strings.TrimSpace(assignee) != "" {
		m["journal"] = assignee
	}
	if strings.TrimSpace(date) != "" {
		m["date"] = date
	}
	if strings.TrimSpace(url) != "" {
		m["url"] = strings.TrimSpace(url)
	}
	return m
}

// buildAddRFCCmd creates the "add rfc" subcommand that adds an RFC by number or manual entry.
func buildAddRFCCmd() *cobra.Command {
	var rfcKeywords string
	c := &cobra.Command{
		Use:   "rfc [rfcNumber]",
		Short: "Add an RFC by number or prompt for manual entry",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
				e, err := rfcpkg.FetchRFC(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return finalizeAndWrite(cmd, e, "rfc", rfcKeywords)
			}
			return manualAdd(cmd, "rfc", parseKeywordsCSV(rfcKeywords))
		},
	}
	c.Flags().StringVar(&rfcKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// buildAddVideoCmd creates the "add video" subcommand that adds a video by YouTube URL or manual entry.
func buildAddVideoCmd() *cobra.Command {
	var ytURL, videoKeywords string
	c := &cobra.Command{
		Use:   "video",
		Short: "Add a video (YouTube URL or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if strings.TrimSpace(ytURL) != "" {
				e, err := youtube.FetchYouTube(cmd.Context(), ytURL)
				if err != nil {
					return err
				}
				return finalizeAndWrite(cmd, e, "video", videoKeywords)
			}
			return manualAdd(cmd, "video", parseKeywordsCSV(videoKeywords))
		},
	}
	c.Flags().StringVar(&ytURL, "youtube", "", "YouTube video URL to fetch via oEmbed")
	c.Flags().StringVar(&videoKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// doAdd is a convenience wrapper for doAddWithKeywords with no extra keywords.
func doAdd(ctx context.Context, typ string, hints map[string]string) error {
	return doAddWithKeywords(ctx, typ, hints, nil)
}

// parseKeywordsCSV splits and normalizes a comma-delimited keywords string.
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

// doAddWithKeywords constructs a minimal entry from hints, validates, writes, and commits it.
func doAddWithKeywords(ctx context.Context, typ string, hints map[string]string, extraKeywords []string) error {
	var e schema.Entry
	e.Type = typ

	title, err := deriveTitle(typ, hints)
	if err != nil {
		return err
	}
	e.APA7.Title = title

	applyJournal(&e, hints)
	applyDate(&e, hints)
	applyURL(&e, hints)
	applyAuthorHint(&e, hints)
	applyIDs(&e, hints)
	ensureAccessedIfURL(&e)

	applyDefaults(&e, typ, extraKeywords)
	applyManualSummary(&e)

	if err := e.Validate(); err != nil {
		return err
	}
	path, err := store.WriteEntry(e)
	if err != nil {
		return err
	}
	if err = commitAndPush([]string{path}, fmt.Sprintf(msgAddCitation, e.ID)); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(os.Stdout, msgWrote, path); err != nil {
		return err
	}
	return nil
}

// deriveTitle returns the title from hints, with a website fallback from URL host.
func deriveTitle(typ string, hints map[string]string) (string, error) {
	title := strings.TrimSpace(hints["title"])
	if typ == "website" && title == "" {
		if u := strings.TrimSpace(hints["url"]); u != "" {
			if pu, err := url.Parse(u); err == nil && pu.Host != "" {
				title = pu.Host
			} else {
				title = u
			}
		}
	}
	if title == "" {
		return "", fmt.Errorf("title is required for %s adds without external metadata", typ)
	}
	return title, nil
}

// applyJournal sets journal/container from hints when provided.
func applyJournal(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["journal"]); v != "" {
		e.APA7.Journal, e.APA7.ContainerTitle = v, v
	}
}

// applyDate sets date and derives year from YYYY prefix if available.
func applyDate(e *schema.Entry, hints map[string]string) {
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
}

// applyURL assigns the URL from hints when provided.
func applyURL(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["url"]); v != "" {
		e.APA7.URL = v
	}
}

// applyAuthorHint parses a single author hint and appends it to the entry.
func applyAuthorHint(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["author"]); v != "" {
		fam, giv := parseAuthor(v)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
}

// applyIDs assigns ISBN/DOI from hints and sets doi.org URL if URL is empty and DOI present.
func applyIDs(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["isbn"]); v != "" {
		e.APA7.ISBN = v
	}
	if v := strings.TrimSpace(hints["doi"]); v != "" {
		e.APA7.DOI = v
		if e.APA7.URL == "" {
			e.APA7.URL = "https://doi.org/" + v
		}
	}
}

// ensureAccessedIfURL sets accessed date when a URL is present and accessed is empty.
func ensureAccessedIfURL(e *schema.Entry) {
	if e.APA7.URL != "" && strings.TrimSpace(e.APA7.Accessed) == "" {
		e.APA7.Accessed = dates.NowISO()
	}
}

// applyDefaults sets id and ensures keywords using provided extras or falls back to the type.
func applyDefaults(e *schema.Entry, typ string, extraKeywords []string) {
	e.ID = schema.NewID()
	if len(extraKeywords) > 0 {
		e.Annotation.Keywords = extraKeywords
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{typ}
	}
}

// applyManualSummary sets a default bibliographic summary based on title and journal.
func applyManualSummary(e *schema.Entry) {
	if strings.TrimSpace(e.APA7.Title) == "" {
		return
	}
	if strings.TrimSpace(e.APA7.Journal) != "" {
		e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s in %s (manually constructed).", e.APA7.Title, e.APA7.Journal)
	} else {
		e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (manually constructed).", e.APA7.Title)
	}
}

// getEnv returns the environment value for key or def if unset.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseAuthor splits a string in the form "Family, Given" into family and given parts.
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

// manualAdd prompts for fields and writes a new entry of the given type.
func manualAdd(cmd *cobra.Command, typ string, extraKeywords []string) error {
	mf, err := collectManualFields(cmd, typ, extraKeywords)
	if err != nil {
		return err
	}
	e, err := buildManualEntry(typ, mf)
	if err != nil {
		return err
	}
	return writeCommitPrint(cmd, e)
}

type manualFields struct {
	title     string
	authorsIn string
	date      string
	url       string
	doi       string
	isbn      string
	journal   string
	publisher string
	summary   string
	keywords  []string
}

func collectManualFields(cmd *cobra.Command, typ string, extraKeywords []string) (manualFields, error) {
	in := cmd.InOrStdin()
	out := cmd.OutOrStdout()
	var mf manualFields
	mf.title = strings.TrimSpace(prompt(cmd, in, out, "Title (required): "))
	if mf.title == "" {
		return manualFields{}, fmt.Errorf("title is required")
	}
	mf.authorsIn = strings.TrimSpace(prompt(cmd, in, out, "Authors (semicolon-separated; use 'Family, Given' or organization name): "))
	mf.date = strings.TrimSpace(prompt(cmd, in, out, "Date (YYYY-MM-DD; optional): "))
	mf.url = strings.TrimSpace(prompt(cmd, in, out, "URL (optional): "))
	switch typ {
	case "article":
		mf.journal = strings.TrimSpace(prompt(cmd, in, out, "Journal/Container (optional): "))
		mf.doi = strings.TrimSpace(prompt(cmd, in, out, "DOI (optional): "))
	case "book":
		mf.publisher = strings.TrimSpace(prompt(cmd, in, out, "Publisher (optional): "))
		mf.isbn = strings.TrimSpace(prompt(cmd, in, out, "ISBN (optional): "))
	case "movie":
		mf.publisher = strings.TrimSpace(prompt(cmd, in, out, "Studio/Publisher (optional): "))
	case "song":
		mf.journal = strings.TrimSpace(prompt(cmd, in, out, "Album/Container (optional): "))
		mf.publisher = strings.TrimSpace(prompt(cmd, in, out, "Label/Publisher (optional): "))
	case "rfc":
		mf.publisher = strings.TrimSpace(prompt(cmd, in, out, "Publisher (default IETF; optional): "))
		if mf.publisher == "" {
			mf.publisher = "IETF"
		}
	}
	mf.summary = strings.TrimSpace(prompt(cmd, in, out, "Summary (required): "))
	if mf.summary == "" {
		mf.summary = fmt.Sprintf("Bibliographic record for %s (manually entered).", mf.title)
	}
	mf.keywords = parseKeywordsCSV(strings.TrimSpace(prompt(cmd, in, out, "Keywords (comma-separated; optional): ")))
	if len(mf.keywords) == 0 {
		mf.keywords = []string{typ}
	}
	if len(extraKeywords) > 0 {
		mf.keywords = append(mf.keywords, extraKeywords...)
	}
	return mf, nil
}

func buildManualEntry(typ string, mf manualFields) (schema.Entry, error) {
	var e schema.Entry
	e.Type = typ
	e.ID = schema.NewID()
	e.APA7.Title = mf.title
	e.APA7.ContainerTitle = mf.journal
	e.APA7.Journal = mf.journal
	e.APA7.Publisher = mf.publisher
	if len(mf.date) >= 4 {
		var y int
		if _, err := fmt.Sscanf(mf.date[:4], "%d", &y); err == nil && y >= 1000 {
			y2 := y
			e.APA7.Year = &y2
		}
	}
	e.APA7.Date = mf.date
	e.APA7.URL = mf.url
	e.APA7.DOI = mf.doi
	e.APA7.ISBN = mf.isbn
	if strings.TrimSpace(e.APA7.URL) != "" {
		e.APA7.Accessed = dates.NowISO()
	}
	for _, name := range splitAuthorsBySemi(mf.authorsIn) {
		fam, giv := parseAuthor(name)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	e.Annotation.Summary = mf.summary
	e.Annotation.Keywords = mf.keywords
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// prompt writes a question and reads a single line input from the given streams.
func prompt(cmd *cobra.Command, in io.Reader, out io.Writer, q string) string {
	// write prompt
	if _, err := fmt.Fprint(out, q); err != nil {
		return ""
	}
	// read line
	br := bufio.NewReader(in)
	s, _ := br.ReadString('\n')
	return strings.TrimRight(s, "\r\n")
}

// splitAuthorsBySemi splits a semicolon-delimited author list and trims each name.
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
