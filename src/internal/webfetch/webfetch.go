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

	"bibliography/src/internal/schema"
)

// HTTPDoer for injection in tests
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var client HTTPDoer = &http.Client{Timeout: 10 * time.Second}

const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

func SetHTTPClient(c HTTPDoer) { client = c }

// PDF extraction extension point for future full-text + XMP integrations
type PDFExtractor interface {
	BuildEntryFromPDF(ctx context.Context, data []byte, sourceURL string) (schema.Entry, error)
}

type defaultPDFExtractor struct{}

func (defaultPDFExtractor) BuildEntryFromPDF(ctx context.Context, data []byte, sourceURL string) (schema.Entry, error) {
	return buildFromPDF(data, sourceURL)
}

var pdfExtractor PDFExtractor = defaultPDFExtractor{}

func SetPDFExtractor(e PDFExtractor) { pdfExtractor = e }

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
	req.Header.Set("User-Agent", chromeUA)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("url fetch: http %d: %s", resp.StatusCode, string(b))
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

	title := firstNonEmpty(og["og:title"], ld.headline, ld.name, metaTitle)
	site := firstNonEmpty(og["og:site_name"], ld.publisher, hostOf(u))
	desc := firstNonEmpty(og["og:description"], ld.description, metaName(body, "description"))
	pub := firstNonEmpty(ld.publisher, site)

	// Authors
	var authors []schema.Author
	for _, n := range ld.authors {
		fam, giv := splitName(n)
		if strings.TrimSpace(fam) != "" {
			authors = append(authors, schema.Author{Family: fam, Given: giv})
		}
	}
	if len(authors) == 0 {
		// try meta name=author (comma separated)
		if a := metaName(body, "author"); a != "" {
			for _, part := range splitAuthors(a) {
				fam, giv := splitName(part)
				if fam != "" {
					authors = append(authors, schema.Author{Family: fam, Given: giv})
				}
			}
		}
	}

	// Date
	date := firstNonEmpty(ld.datePublished, og["article:published_time"], metaName(body, "date"))
	var yearPtr *int
	if y := extractYear(date); y > 0 {
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
	e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
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
		e.ID = schema.Slugify(e.APA7.Title, e.APA7.Year)
	}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

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

func splitName(name string) (family, given string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	if i := strings.Index(name, ","); i >= 0 {
		family = strings.TrimSpace(name[:i])
		given = strings.TrimSpace(name[i+1:])
		return family, toInitials(given)
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		return parts[0], ""
	}
	family = parts[len(parts)-1]
	giv := strings.Join(parts[:len(parts)-1], " ")
	return family, toInitials(giv)
}

func toInitials(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var out []string
	for _, w := range strings.Fields(s) {
		r := []rune(w)
		if len(r) == 0 {
			continue
		}
		out = append(out, strings.ToUpper(string(r[0]))+".")
	}
	return strings.Join(out, " ")
}

func extractYear(s string) int {
	s = strings.TrimSpace(s)
	// find any 4-digit sequence
	for i := 0; i+4 <= len(s); i++ {
		var y int
		if _, err := fmt.Sscanf(s[i:i+4], "%d", &y); err == nil {
			if y >= 1000 && y <= time.Now().Year()+1 {
				return y
			}
		}
	}
	return 0
}

var rePDFTitle = regexp.MustCompile(`(?is)/Title\s*\((.*?)\)`)
var rePDFAuthor = regexp.MustCompile(`(?is)/Author\s*\((.*?)\)`)
var rePDFCreation = regexp.MustCompile(`(?is)/CreationDate\s*\((.*?)\)`)
var reXMPTitle = regexp.MustCompile(`(?is)<dc:title>.*?<rdf:Alt>.*?<rdf:li[^>]*>(.*?)</rdf:li>.*?</dc:title>`)
var reXMPAuthors = regexp.MustCompile(`(?is)<dc:creator>.*?<rdf:Seq>(.*?)</rdf:Seq>.*?</dc:creator>`)
var reXMPAuthorItem = regexp.MustCompile(`(?is)<rdf:li[^>]*>(.*?)</rdf:li>`)
var reDOI = regexp.MustCompile(`(?i)10\.\d{4,9}/[-._;()/:A-Z0-9]+`)

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
	if y := extractYear(date); y > 0 {
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
	e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	if doi != "" {
		e.APA7.DOI = doi
	}
	for _, n := range authorNames {
		fam, giv := splitName(n)
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
		e.ID = schema.Slugify(e.APA7.Title, e.APA7.Year)
	}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func matchFirst(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func pdfUnescape(s string) string {
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func htmlUnescape(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'")
	return r.Replace(s)
}
