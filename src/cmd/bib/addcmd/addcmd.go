package addcmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

    "github.com/spf13/cobra"

    "bibliography/src/internal/booksearch"
    "bibliography/src/internal/dates"
    "bibliography/src/internal/doi"
    moviefetch "bibliography/src/internal/movie"
    rfcpkg "bibliography/src/internal/rfc"
    "bibliography/src/internal/schema"
    songfetch "bibliography/src/internal/song"
    "bibliography/src/internal/store"
    "bibliography/src/internal/summarize"
    youtube "bibliography/src/internal/video"
    "bibliography/src/internal/webfetch"
)

type CommitFunc func(paths []string, message string) error

type Builder struct {
	Commit CommitFunc
}

func New(commit CommitFunc) Builder { return Builder{Commit: commit} }

// AddWithKeywords is an exported convenience wrapper to add an entry using hints
// and optional keywords; used by package main tests and shims.
func AddWithKeywords(ctx context.Context, commit CommitFunc, typ string, hints map[string]string, extraKeywords []string) error {
	return doAddWithKeywords(ctx, commit, typ, hints, extraKeywords)
}

const (
	msgCommaDelimitedKeywords = "comma-delimited keywords to set on the entry"
	msgWrote                  = "wrote %s\n"
	msgAddCitation            = "add citation: %s"
)

// Site returns the "add site" subcommand.
func (b Builder) Site() *cobra.Command {
	var siteKeywords string
	c := &cobra.Command{
		Use:   "site [url]",
		Short: "Add a website by URL or prompt for manual entry",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) >= 1 && strings.TrimSpace(args[0]) != "" {
                store.SetWriteSource("web")
                thisUrl := args[0]
                return doAddWithKeywords(cmd.Context(), b.Commit, "website", map[string]string{"url": thisUrl}, parseKeywordsCSV(siteKeywords))
            }
            store.SetWriteSource("manual")
            return manualAdd(cmd, b.Commit, "website", parseKeywordsCSV(siteKeywords))
        },
    }
	c.Flags().StringVar(&siteKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// Book returns the "add book" subcommand.
func (b Builder) Book() *cobra.Command {
	var bookName, bookAuthor, bookISBN, bookKeywords string
	var bookLookup bool
	c := &cobra.Command{
		Use:   "book",
		Short: "Add a book (flags or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) error {
            if strings.TrimSpace(bookISBN) != "" {
                e, provider, attempts, err := booksearch.LookupBookByISBN(cmd.Context(), bookISBN)
				// Print per-provider attempt status (found/not found)
				for _, a := range attempts {
					status := "status: found"
					if !a.Success {
						status = "status: not found"
					}
					if _, perr := fmt.Fprintf(cmd.OutOrStdout(), "tried: %s: %s\n", a.Provider, status); perr != nil {
						return perr
					}
				}
                if err != nil {
                    return err
                }
                if provider != "" {
                    // Print provider to stdout as requested
                    if _, perr := fmt.Fprintf(cmd.OutOrStdout(), "source: %s\n", provider); perr != nil {
                        return perr
                    }
                    store.SetWriteSource(provider)
                }
                applyKeywordsOverride(&e, bookKeywords)
                return b.writeCommitPrint(cmd, e)
            }
            if strings.TrimSpace(bookName) == "" && strings.TrimSpace(bookAuthor) == "" {
                store.SetWriteSource("manual")
                return manualAdd(cmd, b.Commit, "book", parseKeywordsCSV(bookKeywords))
            }
            // If title/author provided and lookup enabled, try online lookup chain
            if bookLookup && strings.TrimSpace(bookISBN) == "" {
                e, provider, attempts, err := booksearch.LookupBookByTitleAuthor(cmd.Context(), bookName, bookAuthor)
				for _, a := range attempts {
					s := "status: found"
					if !a.Success {
						s = "status: not found"
					}
					if _, perr := fmt.Fprintf(cmd.OutOrStdout(), "tried: %s: %s\n", a.Provider, s); perr != nil {
						return perr
					}
				}
                if err == nil {
                    if provider != "" {
                        if _, perr := fmt.Fprintf(cmd.OutOrStdout(), "source: %s\n", provider); perr != nil {
                            return perr
                        }
                        store.SetWriteSource(provider)
                    }
                    applyKeywordsOverride(&e, bookKeywords)
                    ensureTypeKeyword(&e, "book")
                    return b.writeCommitPrint(cmd, e)
                }
				// fall through to manual/hints if lookup failed
			}
            store.SetWriteSource("manual")
            hints := hintsBook(bookName, bookAuthor, bookISBN)
            return doAddWithKeywords(cmd.Context(), b.Commit, "book", hints, parseKeywordsCSV(bookKeywords))
        },
    }
	c.Flags().StringVar(&bookName, "name", "", "Book title")
	c.Flags().StringVar(&bookAuthor, "author", "", "Author (Family, Given)")
	c.Flags().StringVar(&bookISBN, "isbn", "", "ISBN")
	c.Flags().StringVar(&bookKeywords, "keywords", "", msgCommaDelimitedKeywords)
	c.Flags().BoolVar(&bookLookup, "lookup", false, "Attempt online lookup when title/author are provided")
	return c
}

// Movie returns the "add movie" subcommand.
func (b Builder) Movie() *cobra.Command {
	var movieDate, movieKeywords string
	c := &cobra.Command{
		Use:   "movie [name]",
		Short: "Add a movie (name or manual entry)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) > 0 {
                title := strings.Join(args, " ")
                if e, ok := getMovieEntry(cmd.Context(), title, movieDate); ok {
                    // provider unknown; default to manual for now
                    store.SetWriteSource("manual")
                    applyKeywordsOverride(&e, movieKeywords)
                    ensureTypeKeyword(&e, "movie")
                    return b.writeCommitPrint(cmd, e)
                }
                store.SetWriteSource("manual")
                return doAddWithKeywords(cmd.Context(), b.Commit, "movie", hintsMovie(title, movieDate), parseKeywordsCSV(movieKeywords))
            }
            store.SetWriteSource("manual")
            return manualAdd(cmd, b.Commit, "movie", parseKeywordsCSV(movieKeywords))
        },
    }
	c.Flags().StringVar(&movieDate, "date", "", "release date YYYY-MM-DD")
	c.Flags().StringVar(&movieKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// Song returns the "add song" subcommand.
func (b Builder) Song() *cobra.Command {
	var songArtist, songDate, songKeywords string
	c := &cobra.Command{
		Use:   "song [title]",
		Short: "Add a song (title/artist or manual entry)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
            if len(args) > 0 {
                title := strings.Join(args, " ")
                if e, ok := getSongEntry(cmd.Context(), title, songArtist, songDate); ok {
                    store.SetWriteSource("itunes")
                    applyKeywordsOverride(&e, songKeywords)
                    ensureTypeKeyword(&e, "song")
                    return b.writeCommitPrint(cmd, e)
                }
                store.SetWriteSource("manual")
                return doAddWithKeywords(cmd.Context(), b.Commit, "song", hintsSong(title, songArtist, songDate), parseKeywordsCSV(songKeywords))
            }
            store.SetWriteSource("manual")
            return manualAdd(cmd, b.Commit, "song", parseKeywordsCSV(songKeywords))
        },
    }
	c.Flags().StringVar(&songArtist, "artist", "", "Artist/performer name")
	c.Flags().StringVar(&songDate, "date", "", "release date YYYY-MM-DD")
	c.Flags().StringVar(&songKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// Article returns the "add article" subcommand.
func (b Builder) Article() *cobra.Command {
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
                store.SetWriteSource("doi.org")
                return b.finalizeAndWrite(cmd, e, "article", artKeywords)
            }
            if strings.TrimSpace(artURL) != "" {
                e, err := getArticleByURL(ctx, artURL)
                if err != nil {
                    return err
                }
                store.SetWriteSource("web")
                return b.finalizeAndWrite(cmd, e, "article", artKeywords)
            }
			h := hintsArticle(artTitle, artAuthor, artJournal, artDate)
			if len(h) == 0 {
				return manualAdd(cmd, b.Commit, "article", parseKeywordsCSV(artKeywords))
			}
			return doAddWithKeywords(ctx, b.Commit, "article", h, parseKeywordsCSV(artKeywords))
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

// Patent returns the "add patent" subcommand.
func (b Builder) Patent() *cobra.Command {
	var patURL, patTitle, patInventor, patAssignee, patDate, patKeywords string
	c := &cobra.Command{
		Use:   "patent",
		Short: "Add a patent (flags or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) error {
			h := hintsPatent(patURL, patTitle, patInventor, patAssignee, patDate)
            if len(h) == 0 {
                store.SetWriteSource("manual")
                return manualAdd(cmd, b.Commit, "patent", parseKeywordsCSV(patKeywords))
            }
            store.SetWriteSource("web")
            return doAddWithKeywords(cmd.Context(), b.Commit, "patent", h, parseKeywordsCSV(patKeywords))
        },
    }
	c.Flags().StringVar(&patURL, "url", "", "Patent URL")
	c.Flags().StringVar(&patTitle, "title", "", "Patent title")
	c.Flags().StringVar(&patInventor, "inventor", "", "Inventor name")
	c.Flags().StringVar(&patAssignee, "assignee", "", "Assignee/owner")
	c.Flags().StringVar(&patDate, "date", "", "Filing/publication date")
	c.Flags().StringVar(&patKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// RFC returns the "add rfc" subcommand.
func (b Builder) RFC() *cobra.Command {
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
                store.SetWriteSource("rfc-editor")
                return b.finalizeAndWrite(cmd, e, "rfc", rfcKeywords)
            }
            store.SetWriteSource("manual")
            return manualAdd(cmd, b.Commit, "rfc", parseKeywordsCSV(rfcKeywords))
        },
    }
	c.Flags().StringVar(&rfcKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// Video returns the "add video" subcommand.
func (b Builder) Video() *cobra.Command {
	var ytURL, videoKeywords string
	c := &cobra.Command{
		Use:   "video",
		Short: "Add a video (YouTube URL or manual entry)",
		RunE: func(cmd *cobra.Command, args []string) error {
            if strings.TrimSpace(ytURL) != "" {
                e, err := youtube.FetchYouTube(cmd.Context(), ytURL)
                if err != nil {
                    return err
                }
                store.SetWriteSource("youtube")
                return b.finalizeAndWrite(cmd, e, "video", videoKeywords)
            }
            store.SetWriteSource("manual")
            return manualAdd(cmd, b.Commit, "video", parseKeywordsCSV(videoKeywords))
        },
    }
	c.Flags().StringVar(&ytURL, "youtube", "", "YouTube video URL to fetch via oEmbed")
	c.Flags().StringVar(&videoKeywords, "keywords", "", msgCommaDelimitedKeywords)
	return c
}

// --- helpers previously in add.go ---

func (b Builder) writeCommitPrint(cmd *cobra.Command, e schema.Entry) error {
	path, err := store.WriteEntry(e)
	if err != nil {
		return err
	}
	// Also commit the regenerated BibTeX library.
	if err := b.Commit([]string{path, store.BibFile}, fmt.Sprintf(msgAddCitation, e.ID)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), msgWrote, path)
	return err
}

func applyKeywordsOverride(e *schema.Entry, kwCSV string) {
	if ks := parseKeywordsCSV(kwCSV); len(ks) > 0 {
		e.Annotation.Keywords = ks
	}
}

func ensureTypeKeyword(e *schema.Entry, typ string) {
	if e == nil {
		return
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{typ}
	}
}

func (b Builder) finalizeAndWrite(cmd *cobra.Command, e schema.Entry, typ string, kwCSV string) error {
	applyKeywordsOverride(&e, kwCSV)
	ensureTypeKeyword(&e, typ)
	return b.writeCommitPrint(cmd, e)
}

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

func hintsMovie(title, date string) map[string]string {
	m := map[string]string{"title": title}
	if strings.TrimSpace(date) != "" {
		m["date"] = date
	}
	return m
}

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

func hintsPatent(urlStr, title, inventor, assignee, date string) map[string]string {
	m := map[string]string{}
	if strings.TrimSpace(title) != "" {
		m["title"] = title
	}
	if strings.TrimSpace(inventor) != "" {
		m["author"] = inventor
	}
	if strings.TrimSpace(assignee) != "" {
		m["publisher"] = assignee
	}
	if strings.TrimSpace(date) != "" {
		m["date"] = date
	}
	if strings.TrimSpace(urlStr) != "" {
		m["url"] = strings.TrimSpace(urlStr)
	}
	return m
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

func doAddWithKeywords(ctx context.Context, commit CommitFunc, typ string, hints map[string]string, extraKeywords []string) error {
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
	schema.EnsureAccessedIfURL(&e)
	applyDefaults(&e, typ, extraKeywords)
	applyManualSummary(&e)
	if err := e.Validate(); err != nil {
		return err
	}
	path, err := store.WriteEntry(e)
	if err != nil {
		return err
	}
	if err = commit([]string{path}, fmt.Sprintf(msgAddCitation, e.ID)); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(os.Stdout, msgWrote, path); err != nil {
		return err
	}
	return nil
}

func deriveTitle(typ string, hints map[string]string) (string, error) {
	title := strings.TrimSpace(hints["title"])
	if (typ == "website" || typ == "patent") && title == "" {
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

func applyJournal(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["journal"]); v != "" {
		e.APA7.Journal, e.APA7.ContainerTitle = v, v
	}
}

func applyDate(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["date"]); v != "" {
		e.APA7.Date = v
		if y := dates.YearFromDate(v); y > 0 {
			y2 := y
			e.APA7.Year = &y2
		}
	}
}

func applyURL(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["url"]); v != "" {
		e.APA7.URL = v
	}
}

func applyAuthorHint(e *schema.Entry, hints map[string]string) {
	if v := strings.TrimSpace(hints["author"]); v != "" {
		fam, giv := parseAuthor(v)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
}

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

func applyDefaults(e *schema.Entry, typ string, extraKeywords []string) {
	e.ID = schema.NewID()
	if len(extraKeywords) > 0 {
		e.Annotation.Keywords = extraKeywords
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{typ}
	}
}

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

// manual entry helpers
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

func manualAdd(cmd *cobra.Command, commit CommitFunc, typ string, extraKeywords []string) error {
	mf, err := collectManualFields(cmd, typ, extraKeywords)
	if err != nil {
		return err
	}
	e, err := buildManualEntry(typ, mf)
	if err != nil {
		return err
	}
	path, err := store.WriteEntry(e)
	if err != nil {
		return err
	}
	if err := commit([]string{path, store.BibFile}, fmt.Sprintf(msgAddCitation, e.ID)); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), msgWrote, path)
	return err
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
	if y := dates.YearFromDate(mf.date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
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

func prompt(cmd *cobra.Command, in io.Reader, out io.Writer, q string) string {
	if _, err := fmt.Fprint(out, q); err != nil {
		return ""
	}
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

// external fetch helpers
func getMovieEntry(ctx context.Context, title, date string) (schema.Entry, bool) {
	if e, err := moviefetch.FetchMovie(ctx, title, date); err == nil {
		return e, true
	}
	if e, err := summarize.GenerateMovieFromTitleAndDate(ctx, title, date); err == nil {
		return e, true
	}
	return schema.Entry{}, false
}

func getSongEntry(ctx context.Context, title, artist, date string) (schema.Entry, bool) {
	if e, err := songfetch.FetchSong(ctx, title, artist, date); err == nil {
		return e, true
	}
	if e, err := summarize.GenerateSongFromTitleArtistDate(ctx, title, artist, date); err == nil {
		return e, true
	}
	return schema.Entry{}, false
}

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

func getArticleByURL(ctx context.Context, u string) (schema.Entry, error) {
	e, err := webfetch.FetchArticleByURL(ctx, u)
	if err == nil {
		return e, nil
	}
	if hs, ok := err.(*webfetch.HTTPStatusError); ok && (hs.Status == 401 || hs.Status == 403) {
		if ce, cerr := summarize.GenerateCitationFromURL(ctx, u); cerr == nil {
			return ce, nil
		}
		return schema.Entry{}, fmt.Errorf("access denied (%d) and OpenAI fallback failed: %v", hs.Status, err)
	}
	return schema.Entry{}, err
}
