package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"bibliography/src/internal/dates"
	"bibliography/src/internal/httpx"
	"bibliography/src/internal/names"
	"bibliography/src/internal/sanitize"
	"bibliography/src/internal/schema"
	"bibliography/src/internal/stringsx"
)

var client httpx.Doer = &http.Client{Timeout: 10 * time.Second}

// SetHTTPClient sets the HTTP client used for outbound requests (for tests).
func SetHTTPClient(c httpx.Doer) { client = c }

// PDFExtractor - PDF extraction extension point for future full-text + XMP integrations
type PDFExtractor interface {
	BuildEntryFromPDF(ctx context.Context, data []byte, sourceURL string) (schema.Entry, error)
}

type defaultPDFExtractor struct{}

// BuildEntryFromPDF implements PDFExtractor by delegating to buildFromPDF.
func (defaultPDFExtractor) BuildEntryFromPDF(ctx context.Context, data []byte, sourceURL string) (schema.Entry, error) {
	return buildFromPDF(data, sourceURL)
}

var pdfExtractor PDFExtractor = defaultPDFExtractor{}

// SetPDFExtractor replaces the PDF extraction implementation (for tests/injection).
func SetPDFExtractor(e PDFExtractor) { pdfExtractor = e }

// HTTPStatusError conveys an HTTP status code from fetch failures so callers can branch on it.
type HTTPStatusError struct {
	Status int
	Body   string
}

// Error formats the HTTPStatusError message with status and body snippet.
func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("url fetch: http %d: %s", e.Status, e.Body)
}

// FetchArticleByURL fetches a web page and tries to map it to an APA7 article entry
// using OpenGraph, JSON-LD, and common meta tags.
func FetchArticleByURL(ctx context.Context, raw string) (schema.Entry, error) {
	u := strings.TrimSpace(raw)
	if _, err := url.ParseRequestURI(u); err != nil {
		return schema.Entry{}, fmt.Errorf("invalid url: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "text/html")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, &HTTPStatusError{Status: resp.StatusCode, Body: string(b)}
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return schema.Entry{}, err
	}
	body := string(bodyBytes)

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "pdf") || strings.HasSuffix(strings.ToLower(u), ".pdf") {
		return pdfExtractor.BuildEntryFromPDF(ctx, bodyBytes, u)
	}

	og, metaTitle := parseOpenGraphAndTitle(body)
	ld := parseJSONLDArticle(body)

	title := stringsx.FirstNonEmpty(og["og:title"], ld.headline, ld.name, metaTitle)
	site := stringsx.FirstNonEmpty(og["og:site_name"], ld.publisher, hostOf(u))
	desc := stringsx.FirstNonEmpty(og["og:description"], ld.description, metaName(body, "description"))
	pub := stringsx.FirstNonEmpty(ld.publisher, site)

	// Authors
	var authors []schema.Author
	for _, n := range ld.authors {
		fam, giv := names.Split(n)
		if strings.TrimSpace(fam) != "" {
			authors = append(authors, schema.Author{Family: fam, Given: giv})
		}
	}
	if len(authors) == 0 {
		// try meta name=author (comma separated)
		if a := metaName(body, "author"); a != "" {
			for _, part := range splitAuthors(a) {
				fam, giv := names.Split(part)
				if fam != "" {
					authors = append(authors, schema.Author{Family: fam, Given: giv})
				}
			}
		}
	}

	// Date
	date := stringsx.FirstNonEmpty(ld.datePublished, og["article:published_time"], metaName(body, "date"))
	var yearPtr *int
	if y := dates.ExtractYear(date); y > 0 {
		y2 := y
		yearPtr = &y2
	}
	if len(date) >= 10 {
		date = date[:10]
	} else {
		date = ""
	}

	e := schema.Entry{Type: "article"}
	e.APA7.Title = title
	e.APA7.ContainerTitle = site
	e.APA7.Publisher = pub
	if yearPtr != nil {
		e.APA7.Year = yearPtr
	}
	if date != "" {
		e.APA7.Date = date
	}
	e.APA7.URL = u
	e.APA7.Accessed = dates.NowISO()
	e.APA7.Authors = authors
	if strings.TrimSpace(desc) != "" {
		e.Annotation.Summary = desc
	} else if title != "" {
		e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s on %s.", title, site)
	} else {
		e.Annotation.Summary = "Web article"
	}
	e.Annotation.Keywords = []string{"article"}
	if strings.TrimSpace(e.ID) == "" {
		e.ID = schema.NewID()
	}
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// firstNonEmpty removed; using stringsx.FirstNonEmpty

// hostOf returns the hostname for a URL without a leading "www.".
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Host)
	return strings.TrimPrefix(h, "www.")
}

var reTitle = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
var reMetaProperty = regexp.MustCompile(`(?is)<meta[^>]*?property\s*=\s*"([^"]+)"[^>]*?content\s*=\s*"([^"]*)"[^>]*>`)
var reMetaName = regexp.MustCompile(`(?is)<meta[^>]*?name\s*=\s*"([^"]+)"[^>]*?content\s*=\s*"([^"]*)"[^>]*>`)

// parseOpenGraphAndTitle extracts OpenGraph/meta properties and the <title>.
func parseOpenGraphAndTitle(body string) (map[string]string, string) {
	og := map[string]string{}
	for _, m := range reMetaProperty.FindAllStringSubmatch(body, -1) {
		prop := strings.ToLower(strings.TrimSpace(m[1]))
		content := strings.TrimSpace(m[2])
		if strings.HasPrefix(prop, "og:") || strings.HasPrefix(prop, "article:") {
			og[prop] = content
		}
	}
	// also capture some name-based meta
	for _, m := range reMetaName.FindAllStringSubmatch(body, -1) {
		name := strings.ToLower(strings.TrimSpace(m[1]))
		content := strings.TrimSpace(m[2])
		if name == "author" && og["author"] == "" {
			og["author"] = content
		}
		if name == "description" && og["og:description"] == "" {
			og["og:description"] = content
		}
	}
	title := ""
	if m := reTitle.FindStringSubmatch(body); len(m) == 2 {
		title = strings.TrimSpace(m[1])
	}
	return og, title
}

var reLDJSON = regexp.MustCompile(`(?is)<script[^>]+type=\"application/ld\+json\"[^>]*>(.*?)</script>`)

type ldArticle struct {
	Headline      string `json:"headline"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	DatePublished string `json:"datePublished"`
	Author        any    `json:"author"`
	Publisher     any    `json:"publisher"`
}

type simplifiedLD struct {
	headline, name, description, datePublished, publisher string
	authors                                               []string
}

// parseJSONLDArticle extracts a simplified article from JSON-LD if present.
func parseJSONLDArticle(body string) simplifiedLD {
	m := reLDJSON.FindStringSubmatch(body)
	if len(m) != 2 {
		return simplifiedLD{}
	}
	payload := strings.TrimSpace(m[1])
	// Could be array or object
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.UseNumber()
	var anyv any
	if err := dec.Decode(&anyv); err != nil {
		return simplifiedLD{}
	}
	// Find first object with @type Article/NewsArticle
	var obj map[string]any
	switch t := anyv.(type) {
	case []any:
		for _, it := range t {
			if o, ok := it.(map[string]any); ok {
				if hasArticleType(o["@type"]) {
					obj = o
					break
				}
			}
		}
	case map[string]any:
		obj = t
	}
	if obj == nil {
		return simplifiedLD{}
	}
	var out simplifiedLD
	out.headline, _ = obj["headline"].(string)
	out.name, _ = obj["name"].(string)
	out.description, _ = obj["description"].(string)
	out.datePublished, _ = obj["datePublished"].(string)
	out.publisher = pickName(obj["publisher"])
	// authors
	out.authors = extractAuthors(obj["author"])
	return out
}

// hasArticleType reports whether a JSON-LD @type contains an Article flavor.
func hasArticleType(v any) bool {
	switch t := v.(type) {
	case string:
		tv := strings.ToLower(t)
		return strings.Contains(tv, "article")
	case []any:
		for _, it := range t {
			if hasArticleType(it) {
				return true
			}
		}
	}
	return false
}

// pickName returns a name string from either a string or {name: "..."} map.
func pickName(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]any); ok {
		if n, ok := m["name"].(string); ok {
			return n
		}
	}
	return ""
}

// extractAuthors returns author name strings from common JSON-LD shapes.
func extractAuthors(v any) []string {
	var out []string
	switch t := v.(type) {
	case string:
		out = append(out, t)
	case []any:
		for _, it := range t {
			if s, ok := it.(string); ok {
				out = append(out, s)
				continue
			}
			if m, ok := it.(map[string]any); ok {
				if n, ok := m["name"].(string); ok {
					out = append(out, n)
				}
			}
		}
	case map[string]any:
		if n, ok := t["name"].(string); ok {
			out = append(out, n)
		}
	}
	return out
}

// metaName finds a meta tag by name and returns its content value.
func metaName(body string, name string) string {
	// simple search
	lower := strings.ToLower(body)
	key := fmt.Sprintf("name=\"%s\"", strings.ToLower(name))
	idx := strings.Index(lower, key)
	if idx == -1 {
		return ""
	}
	// find content="..."
	rest := body[idx:]
	ci := strings.Index(strings.ToLower(rest), "content=")
	if ci == -1 {
		return ""
	}
	rest = rest[ci+8:]
	// skip optional spaces and quotes
	rest = strings.TrimLeft(rest, " \t\r\n")
	if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
		quote := rest[0]
		rest = rest[1:]
		if j := strings.IndexByte(rest, quote); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	// fallback: read until space/>
	for i, ch := range rest {
		if ch == ' ' || ch == '>' {
			return strings.TrimSpace(rest[:i])
		}
	}
	return ""
}

// splitAuthors splits an author string by comma or " and ".
func splitAuthors(s string) []string {
	// split on comma or ' and '
	s = strings.ReplaceAll(s, " and ", ",")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitName, toInitials, extractYear removed; use names.Split and dates.ExtractYear

var rePDFTitle = regexp.MustCompile(`(?is)/Title\s*\((.*?)\)`)
var rePDFAuthor = regexp.MustCompile(`(?is)/Author\s*\((.*?)\)`)
var rePDFCreation = regexp.MustCompile(`(?is)/CreationDate\s*\((.*?)\)`)
var reXMPTitle = regexp.MustCompile(`(?is)<dc:title>.*?<rdf:Alt>.*?<rdf:li[^>]*>(.*?)</rdf:li>.*?</dc:title>`)
var reXMPAuthors = regexp.MustCompile(`(?is)<dc:creator>.*?<rdf:Seq>(.*?)</rdf:Seq>.*?</dc:creator>`)
var reXMPAuthorItem = regexp.MustCompile(`(?is)<rdf:li[^>]*>(.*?)</rdf:li>`)
var reDOI = regexp.MustCompile(`(?i)10\.\d{4,9}/[-._;()/:A-Z0-9]+`)

// buildFromPDF parses minimal metadata from raw PDF bytes and constructs an Entry.
func buildFromPDF(b []byte, sourceURL string) (schema.Entry, error) {
	s := string(b)
	// Title
	title := matchFirst(rePDFTitle, s)
	if title == "" {
		title = matchFirst(reXMPTitle, s)
	}
	title = pdfUnescape(title)
	// Authors
	var authorNames []string
	if block := matchFirst(reXMPAuthors, s); block != "" {
		for _, m := range reXMPAuthorItem.FindAllStringSubmatch(block, -1) {
			name := strings.TrimSpace(htmlUnescape(m[1]))
			if name != "" {
				authorNames = append(authorNames, name)
			}
		}
	}
	if len(authorNames) == 0 {
		a := pdfUnescape(matchFirst(rePDFAuthor, s))
		if a != "" {
			authorNames = splitAuthors(a)
		}
	}
	// Date/Year
	date := pdfUnescape(matchFirst(rePDFCreation, s))
	var yearPtr *int
	if y := dates.ExtractYear(date); y > 0 {
		y2 := y
		yearPtr = &y2
	}

	// DOI
	doi := matchFirst(reDOI, strings.ToUpper(s))

	host := hostOf(sourceURL)
	e := schema.Entry{Type: "article"}
	e.APA7.Title = title
	e.APA7.ContainerTitle = host
	e.APA7.Publisher = host
	if yearPtr != nil {
		e.APA7.Year = yearPtr
	}
	if len(date) >= 10 {
		e.APA7.Date = date[:10]
	}
	e.APA7.URL = sourceURL
	e.APA7.Accessed = dates.NowISO()
	if doi != "" {
		e.APA7.DOI = doi
	}
	for _, n := range authorNames {
		fam, giv := names.Split(n)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	if strings.TrimSpace(e.APA7.Title) == "" {
		e.APA7.Title = host
	}
	// Summary
	if doi != "" {
		e.Annotation.Summary = fmt.Sprintf("PDF article from %s with DOI %s.", host, doi)
	} else if e.APA7.Title != "" {
		e.Annotation.Summary = fmt.Sprintf("PDF article: %s (from %s).", e.APA7.Title, host)
	} else {
		e.Annotation.Summary = fmt.Sprintf("PDF article from %s.", host)
	}
	e.Annotation.Keywords = []string{"article"}
	if strings.TrimSpace(e.ID) == "" {
		e.ID = schema.NewID()
	}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// matchFirst returns the first submatch group or empty string.
func matchFirst(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// pdfUnescape unescapes common PDF literal string escapes.
func pdfUnescape(s string) string {
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// htmlUnescape decodes a minimal set of HTML entities.
func htmlUnescape(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'")
	return r.Replace(s)
}
