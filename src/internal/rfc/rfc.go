package rfc

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"bibliography/src/internal/schema"
)

// HTTPDoer allows injecting an HTTP client for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var client HTTPDoer = &http.Client{Timeout: 10 * time.Second}

const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// SetHTTPClient swaps the http client for tests.
func SetHTTPClient(c HTTPDoer) { client = c }

// Minimal subset of RFC Editor XML structures we care about
type rfcXML struct {
	XMLName xml.Name `xml:"rfc"`
	Front   frontXML `xml:"front"`
}

type frontXML struct {
	Title       string          `xml:"title"`
	Authors     []authorXML     `xml:"author"`
	Date        dateXML         `xml:"date"`
	SeriesInfos []seriesInfoXML `xml:"seriesInfo"`
	Abstract    abstractXML     `xml:"abstract"`
}

type authorXML struct {
	FullName string     `xml:"fullname,attr"`
	Name     authorName `xml:"name"`
}

type authorName struct {
	Initials string `xml:"initials"`
	Given    string `xml:"given"`
	Surname  string `xml:"surname"`
}

type dateXML struct {
	Month string `xml:"month,attr"`
	Year  string `xml:"year,attr"`
	Day   string `xml:"day,attr"`
}

type seriesInfoXML struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type abstractXML struct {
	Paras []string `xml:"t"`
}

// FetchRFC fetches an RFC HTML page and maps it into a schema.Entry with type "rfc".
// spec may be "rfc5424", "RFC5424", or just "5424".
func FetchRFC(ctx context.Context, spec string) (schema.Entry, error) {
	num := normalizeRFCNumber(spec)
	if num == "" {
		return schema.Entry{}, fmt.Errorf("invalid RFC spec: %s", spec)
	}
	// Try BibTeX via Datatracker first
	if e, err := fetchRFCFromBibtex(ctx, num); err == nil {
		return e, nil
	}
	xmlURL := fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%s.xml", num)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xmlURL, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("User-Agent", chromeUA)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Fallback to datatracker HTML for legacy RFCs without XML
		if resp.StatusCode == http.StatusNotFound {
			return fetchRFCFromDatatracker(ctx, num)
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("rfc: http %d: %s", resp.StatusCode, string(b))
	}
	var doc rfcXML
	dec := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if err := dec.Decode(&doc); err != nil {
		// Fallback to datatracker HTML if xml parse fails
		return fetchRFCFromDatatracker(ctx, num)
	}

	title := strings.TrimSpace(doc.Front.Title)
	if title == "" {
		title = "RFC " + num
	}
	// Date
	var yearPtr *int
	date := ""
	if y := toInt(doc.Front.Date.Year); y > 0 {
		y2 := y
		yearPtr = &y2
		if m := monthToNum(doc.Front.Date.Month); m > 0 {
			d := 1
			if dd := toInt(doc.Front.Date.Day); dd > 0 {
				d = dd
			}
			date = fmt.Sprintf("%04d-%02d-%02d", y, m, d)
		}
	}
	// Authors
	var authors []schema.Author
	for _, a := range doc.Front.Authors {
		fam := strings.TrimSpace(a.Name.Surname)
		giv := strings.TrimSpace(a.Name.Given)
		if fam == "" && a.FullName != "" {
			// split fullname
			fam, giv = splitFullName(a.FullName)
		}
		if fam == "" && a.Name.Initials != "" {
			// fallback to initials as given and use fullname as family
			if a.FullName != "" {
				fam = a.FullName
			}
		}
		if fam == "" {
			continue
		}
		authors = append(authors, schema.Author{Family: fam, Given: toInitials(giv)})
	}
	// Series info
	rfcLabel := "RFC " + num
	doi := ""
	for _, si := range doc.Front.SeriesInfos {
		name := strings.ToUpper(strings.TrimSpace(si.Name))
		if name == "RFC" && strings.TrimSpace(si.Value) != "" {
			rfcLabel = "RFC " + strings.TrimSpace(si.Value)
		}
		if name == "DOI" && strings.TrimSpace(si.Value) != "" {
			doi = strings.TrimSpace(si.Value)
		}
	}
	if doi == "" {
		doi = fmt.Sprintf("10.17487/RFC%s", num)
	}

	e := schema.Entry{Type: "rfc"}
	e.ID = "rfc" + num
	e.APA7.Title = title
	e.APA7.ContainerTitle = rfcLabel
	e.APA7.Publisher = "Internet Engineering Task Force"
	e.APA7.URL = fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%s.html", num)
	e.APA7.BibTeXURL = fmt.Sprintf("https://datatracker.ietf.org/doc/rfc%s/bibtex/", num)
	e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	if yearPtr != nil {
		e.APA7.Year = yearPtr
	}
	if date != "" {
		e.APA7.Date = date
	}
	if doi != "" {
		e.APA7.DOI = doi
	}
	e.APA7.Authors = authors
	if abs := strings.TrimSpace(strings.Join(doc.Front.Abstract.Paras, "\n\n")); abs != "" {
		e.Annotation.Summary = sanitizeAbstract(abs)
	} else {
		e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s).", e.APA7.Title, rfcLabel)
	}
	e.Annotation.Keywords = []string{"rfc", "ietf"}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func normalizeRFCNumber(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "rfc") {
		s = s[3:]
	}
	// strip non-digits
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	return string(out)
}

func monthToNum(m string) int {
	m = strings.ToLower(strings.TrimSpace(m))
	switch m {
	case "january":
		return 1
	case "february":
		return 2
	case "march":
		return 3
	case "april":
		return 4
	case "may":
		return 5
	case "june":
		return 6
	case "july":
		return 7
	case "august":
		return 8
	case "september":
		return 9
	case "october":
		return 10
	case "november":
		return 11
	case "december":
		return 12
	case "jan":
		return 1
	case "feb":
		return 2
	case "mar":
		return 3
	case "apr":
		return 4
	case "jun":
		return 6
	case "jul":
		return 7
	case "aug":
		return 8
	case "sep":
		return 9
	case "oct":
		return 10
	case "nov":
		return 11
	case "dec":
		return 12
	default:
		return 0
	}
}

func toInt(s string) int {
	var v int
	_, _ = fmt.Sscanf(s, "%d", &v)
	return v
}

func toInitials(given string) string {
	given = strings.TrimSpace(given)
	if given == "" {
		return ""
	}
	parts := strings.Fields(given)
	var out []string
	for _, p := range parts {
		r := []rune(p)
		if len(r) == 0 {
			continue
		}
		if len(r) == 1 && r[0] >= 'A' && r[0] <= 'Z' {
			out = append(out, string(r[0])+".")
		} else if strings.HasSuffix(p, ".") {
			out = append(out, p)
		} else {
			out = append(out, strings.ToUpper(string(r[0]))+".")
		}
	}
	return strings.Join(out, " ")
}

// --- Fallback: datatracker HTML parser for legacy RFCs without XML ---

var (
	reTitle     = regexp.MustCompile(`(?is)<title>\s*RFC\s*(\d+)\s*-\s*([^<]+)</title>`)
	reDOI       = regexp.MustCompile(`(?i)DOI\s*:\s*([0-9.]+/RFC\d+)`)
	reMonthYear = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{4})\b`)
	reName      = regexp.MustCompile(`\b([A-Z](?:\.|[a-z]+)(?:\s+[A-Z]\\.)*)\s+([A-Z][a-zA-Z\-']+)\b`)
)

func fetchRFCFromDatatracker(ctx context.Context, num string) (schema.Entry, error) {
	url := fmt.Sprintf("https://datatracker.ietf.org/doc/html/rfc%s", num)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		return schema.Entry{}, fmt.Errorf("rfc (fallback): http %d: %s", resp.StatusCode, string(b))
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return schema.Entry{}, err
	}
	body := string(bodyBytes)

	title := "RFC " + num
	if m := reTitle.FindStringSubmatch(body); len(m) == 3 {
		title = strings.TrimSpace(m[2])
	}
	var yearPtr *int
	if m := reMonthYear.FindStringSubmatch(body); len(m) == 3 {
		y := toInt(m[2])
		if y > 0 {
			y2 := y
			yearPtr = &y2
		}
	}
	// Avoid unreliable author extraction from HTML; leave authors empty
	var authors []schema.Author
	doi := ""
	if m := reDOI.FindStringSubmatch(body); len(m) == 2 {
		doi = strings.TrimSpace(m[1])
	}
	if doi == "" {
		doi = fmt.Sprintf("10.17487/RFC%s", num)
	}

	e := schema.Entry{Type: "rfc"}
	e.ID = "rfc" + num
	e.APA7.Title = title
	e.APA7.ContainerTitle = "RFC " + num
	e.APA7.Publisher = "Internet Engineering Task Force"
	e.APA7.URL = url
	e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	if yearPtr != nil {
		e.APA7.Year = yearPtr
	}
	if doi != "" {
		e.APA7.DOI = doi
	}
	e.APA7.Authors = authors
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (RFC %s).", e.APA7.Title, num)
	e.Annotation.Keywords = []string{"rfc", "ietf"}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// --- BibTeX fetch and parse ---

func fetchRFCFromBibtex(ctx context.Context, num string) (schema.Entry, error) {
	url := fmt.Sprintf("https://datatracker.ietf.org/doc/rfc%s/bibtex/", num)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "text/plain, text/html")
	req.Header.Set("User-Agent", chromeUA)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("bibtex: http %d: %s", resp.StatusCode, string(b))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return schema.Entry{}, err
	}
	raw := string(b)
	// Extract needed fields allowing multiline brace/quote values
	titleVal := getBibField(raw, "title")
	authorVal := getBibField(raw, "author")
	publisherVal := getBibField(raw, "publisher")
	urlVal := getBibField(raw, "url")
	doiVal := getBibField(raw, "doi")
	yearVal := getBibField(raw, "year")
	monthVal := getBibField(raw, "month")
	if strings.TrimSpace(titleVal) == "" {
		return schema.Entry{}, fmt.Errorf("bibtex: parse failed")
	}
	// Map fields to entry
	e := schema.Entry{Type: "rfc"}
	e.ID = "rfc" + num
	e.APA7.Title = cleanBibVal(titleVal)
	e.APA7.ContainerTitle = "RFC " + num
	if v := strings.TrimSpace(publisherVal); v != "" {
		e.APA7.Publisher = v
	} else {
		e.APA7.Publisher = "RFC Editor"
	}
	if v := strings.TrimSpace(urlVal); v != "" {
		e.APA7.URL = v
	} else {
		e.APA7.URL = fmt.Sprintf("https://www.rfc-editor.org/rfc/rfc%s.html", num)
	}
	e.APA7.BibTeXURL = fmt.Sprintf("https://datatracker.ietf.org/doc/rfc%s/bibtex/", num)
	e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
	if y := toInt(yearVal); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	if m := monthToNum(monthVal); m > 0 {
		y := 0
		if e.APA7.Year != nil {
			y = *e.APA7.Year
		}
		if y > 0 {
			e.APA7.Date = fmt.Sprintf("%04d-%02d-01", y, m)
		}
	}
	if doi := strings.TrimSpace(doiVal); doi != "" {
		e.APA7.DOI = doi
	} else {
		e.APA7.DOI = fmt.Sprintf("10.17487/RFC%s", num)
	}
	if auth := strings.TrimSpace(authorVal); auth != "" {
		// Normalize whitespace and split on 'and' (case-insensitive)
		auth = normalizeSpaces(auth)
		parts := reAnd.Split(auth, -1)
		for _, p := range parts {
			p = cleanBibVal(strings.TrimSpace(p))
			if p == "" {
				continue
			}
			fam, giv := splitAuthorNameBib(p)
			if fam == "" {
				continue
			}
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: toInitials(giv)})
		}
	}
	// Prefer abstract for annotation summary
	if abs := strings.TrimSpace(getBibField(raw, "abstract")); abs != "" {
		e.Annotation.Summary = sanitizeAbstract(abs)
	} else {
		e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (RFC %s).", e.APA7.Title, num)
	}
	e.Annotation.Keywords = []string{"rfc", "ietf"}
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

var (
	reFieldBraced = regexp.MustCompile(`(?mi)^\s*([a-zA-Z]+)\s*=\s*\{([^}]*)\}\s*,?\s*$`)
	reFieldQuoted = regexp.MustCompile(`(?mi)^\s*([a-zA-Z]+)\s*=\s*\"([^\"]*)\"\s*,?\s*$`)
	reFieldBare   = regexp.MustCompile(`(?mi)^\s*([a-zA-Z]+)\s*=\s*([a-zA-Z]+)\s*,?\s*$`)
	reAnd         = regexp.MustCompile(`(?i)\s+and\s+`)
	reSpaces      = regexp.MustCompile(`\s+`)
	reFieldAny    = regexp.MustCompile(`(?mi)^\s*([a-zA-Z]+)\s*=\s*(.+?)\s*,?\s*$`)
	reGetBraced   = regexp.MustCompile(`(?is)\b([a-zA-Z]+)\s*=\s*\{([^}]*)\}`)
	reGetQuoted   = regexp.MustCompile(`(?is)\b([a-zA-Z]+)\s*=\s*\"([^\"]*)\"`)
	reGetBare     = regexp.MustCompile(`(?is)\b([a-zA-Z]+)\s*=\s*([^\s,}]+)`)
)

func parseBibtexFields(s string) map[string]string {
	fields := map[string]string{}
	// Extract only the entry body if wrapped (handle HTML wrappers)
	s = extractBibtexBlock(s)
	lines := strings.Split(s, "\n")
	for _, ln := range lines {
		if m := reFieldBraced.FindStringSubmatch(ln); len(m) == 3 {
			k := strings.ToLower(strings.TrimSpace(m[1]))
			v := strings.TrimSpace(m[2])
			fields[k] = v
			continue
		}
		if m := reFieldQuoted.FindStringSubmatch(ln); len(m) == 3 {
			k := strings.ToLower(strings.TrimSpace(m[1]))
			v := strings.TrimSpace(m[2])
			fields[k] = v
			continue
		}
		if m := reFieldBare.FindStringSubmatch(ln); len(m) == 3 {
			k := strings.ToLower(strings.TrimSpace(m[1]))
			v := strings.TrimSpace(m[2])
			fields[k] = v
			continue
		}
		if m := reFieldAny.FindStringSubmatch(ln); len(m) == 3 {
			k := strings.ToLower(strings.TrimSpace(m[1]))
			v := strings.TrimSpace(m[2])
			// strip wrapping braces or quotes if present
			v = strings.Trim(v, ",")
			v = strings.Trim(v, "{}")
			v = strings.Trim(v, `"`)
			fields[k] = v
			continue
		}
	}
	return fields
}

func cleanBibVal(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "{}")
	return s
}

func getBibField(s, key string) string {
	// search for braced value (multiline)
	rb := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(key) + `\s*=\s*\{([^}]*)\}`)
	if m := rb.FindStringSubmatch(s); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	rq := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(key) + `\s*=\s*\"([^\"]*)\"`)
	if m := rq.FindStringSubmatch(s); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	rbare := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(key) + `\s*=\s*([^\s,}]+)`)
	if m := rbare.FindStringSubmatch(s); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func extractBibtexBlock(s string) string {
	i := strings.Index(s, "@")
	if i < 0 {
		return s
	}
	s = s[i:]
	// find matching braces for the first entry
	// find first '{'
	j := strings.Index(s, "{")
	if j < 0 {
		return s
	}
	depth := 1
	k := j + 1
	for k < len(s) {
		switch s[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:k+1]
			}
		}
		k++
	}
	return s
}

func splitAuthorNameBib(s string) (family, given string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if strings.Contains(s, ",") {
		parts := strings.SplitN(s, ",", 2)
		family = strings.TrimSpace(parts[0])
		given = ""
		if len(parts) > 1 {
			given = strings.TrimSpace(parts[1])
		}
		return family, given
	}
	parts := strings.Fields(s)
	if len(parts) == 1 {
		return parts[0], ""
	}
	family = parts[len(parts)-1]
	given = strings.Join(parts[:len(parts)-1], " ")
	return family, given
}

func normalizeSpaces(s string) string { return reSpaces.ReplaceAllString(strings.TrimSpace(s), " ") }

func splitFullName(s string) (family, given string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	parts := strings.Fields(s)
	if len(parts) == 1 {
		return parts[0], ""
	}
	family = parts[len(parts)-1]
	given = strings.Join(parts[:len(parts)-1], " ")
	return family, given
}

func sanitizeAbstract(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	// collapse multiple blank lines
	out := make([]string, 0, len(lines))
	lastBlank := false
	for _, ln := range lines {
		if ln == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			out = append(out, "")
			continue
		}
		lastBlank = false
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
