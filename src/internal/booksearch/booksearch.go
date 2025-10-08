package booksearch

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bibliography/src/internal/dates"
	"bibliography/src/internal/httpx"
	"bibliography/src/internal/names"
	"bibliography/src/internal/openlibrary"
	"bibliography/src/internal/sanitize"
	"bibliography/src/internal/schema"
)

// client is the HTTP client used by this package; replaceable in tests.
var client httpx.Doer = &http.Client{Timeout: 12 * time.Second}

// SetHTTPClient allows tests to inject a fake http client.
func SetHTTPClient(c httpx.Doer) { client = c }

// Attempt captures a single provider attempt outcome.
type Attempt struct {
	Provider string
	Success  bool
	Error    string
}

// LookupBookByISBN attempts to fetch book metadata from a sequence of providers.
// Order:
//  1. OpenLibrary (includes internal Google Books fallback)
//  2. Crossref REST API
//  3. OCLC Classify (WorldCat)
//  4. British National Bibliography (BNB) SPARQL
//  5. openBD (Japan)
//  6. US Library of Congress
//  7. OpenAI (last resort)
func LookupBookByISBN(ctx context.Context, isbn string) (schema.Entry, string, []Attempt, error) {
	attempts := []Attempt{}
	// 1) OpenLibrary (already falls back to Google Books internally)
	if e, err := openlibrary.FetchBookByISBN(ctx, isbn); err == nil {
		attempts = append(attempts, Attempt{Provider: "openlibrary", Success: true})
		return e, "openlibrary", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "openlibrary", Success: false, Error: err.Error()})
	}
	// 2) Crossref
	if e, err := fetchCrossrefByISBN(ctx, isbn); err == nil {
		attempts = append(attempts, Attempt{Provider: "crossref", Success: true})
		return e, "crossref", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "crossref", Success: false, Error: err.Error()})
	}
	// 3) OCLC Classify API (WorldCat)
	if e, err := fetchOCLCClassifyByISBN(ctx, isbn); err == nil {
		attempts = append(attempts, Attempt{Provider: "oclc", Success: true})
		return e, "oclc", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "oclc", Success: false, Error: err.Error()})
	}
	// 4) BNB SPARQL
	if e, err := fetchBNBByISBN(ctx, isbn); err == nil {
		attempts = append(attempts, Attempt{Provider: "bnb", Success: true})
		return e, "bnb", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "bnb", Success: false, Error: err.Error()})
	}
	// 5) openBD
	if e, err := fetchOpenBDByISBN(ctx, isbn); err == nil {
		attempts = append(attempts, Attempt{Provider: "openbd", Success: true})
		return e, "openbd", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "openbd", Success: false, Error: err.Error()})
	}
	// 6) Library of Congress
	if e, err := fetchLoCByISBN(ctx, isbn); err == nil {
		attempts = append(attempts, Attempt{Provider: "loc", Success: true})
		return e, "loc", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "loc", Success: false, Error: err.Error()})
	}
	// No OpenAI last-resort for books; return an error including attempts
	return schema.Entry{}, "", attempts, fmt.Errorf("no providers returned data for ISBN %s", strings.TrimSpace(isbn))
}

// LookupBookByTitleAuthor tries to find a book using title and author strings.
// Order: OpenLibrary Search -> Google Books -> Crossref. Returns attempts trace.
func LookupBookByTitleAuthor(ctx context.Context, title, author string) (schema.Entry, string, []Attempt, error) {
	attempts := []Attempt{}
	// 1) OpenLibrary Search API
	if e, err := searchOpenLibrary(ctx, title, author); err == nil {
		attempts = append(attempts, Attempt{Provider: "openlibrary-search", Success: true})
		return e, "openlibrary", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "openlibrary-search", Success: false, Error: err.Error()})
	}
	// 2) Google Books Search
	if e, err := searchGoogleBooks(ctx, title, author); err == nil {
		attempts = append(attempts, Attempt{Provider: "googlebooks", Success: true})
		return e, "googlebooks", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "googlebooks", Success: false, Error: err.Error()})
	}
	// 3) Crossref query
	if e, err := searchCrossref(ctx, title, author); err == nil {
		attempts = append(attempts, Attempt{Provider: "crossref", Success: true})
		return e, "crossref", attempts, nil
	} else {
		attempts = append(attempts, Attempt{Provider: "crossref", Success: false, Error: err.Error()})
	}
	return schema.Entry{}, "", attempts, fmt.Errorf("no providers returned data for title/author")
}

func searchOpenLibrary(ctx context.Context, title, author string) (schema.Entry, error) {
	v := url.Values{}
	if strings.TrimSpace(title) != "" {
		v.Set("title", title)
	}
	if strings.TrimSpace(author) != "" {
		v.Set("author", author)
	}
	v.Set("limit", "1")
	endpoint := "https://openlibrary.org/search.json?" + v.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Accept", "application/json")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("openlibrary: http %d: %s", resp.StatusCode, string(b))
	}
	var r struct {
		Docs []struct {
			Title      string   `json:"title"`
			AuthorName []string `json:"author_name"`
			Publisher  []string `json:"publisher"`
			FirstYear  int      `json:"first_publish_year"`
			Key        string   `json:"key"`
		} `json:"docs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return schema.Entry{}, err
	}
	if len(r.Docs) == 0 {
		return schema.Entry{}, fmt.Errorf("openlibrary: no results")
	}
	d := r.Docs[0]
	var e schema.Entry
	e.Type = "book"
	e.ID = schema.NewID()
	e.APA7.Title = strings.TrimSpace(d.Title)
	if len(d.AuthorName) > 0 {
		fam, giv := splitAuthor(d.AuthorName[0])
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	if len(d.Publisher) > 0 {
		e.APA7.Publisher = strings.TrimSpace(d.Publisher[0])
	}
	if d.FirstYear > 0 {
		y := d.FirstYear
		e.APA7.Year = &y
	}
	if strings.TrimSpace(d.Key) != "" {
		e.APA7.URL = "https://openlibrary.org" + d.Key
		e.APA7.Accessed = dates.NowISO()
	}
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from OpenLibrary search.", e.APA7.Title)
	e.Annotation.Keywords = []string{"book"}
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func searchGoogleBooks(ctx context.Context, title, author string) (schema.Entry, error) {
	qparts := []string{}
	if strings.TrimSpace(title) != "" {
		qparts = append(qparts, "intitle:"+title)
	}
	if strings.TrimSpace(author) != "" {
		qparts = append(qparts, "inauthor:"+author)
	}
	v := url.Values{}
	v.Set("q", strings.Join(qparts, "+"))
	v.Set("maxResults", "1")
	endpoint := "https://www.googleapis.com/books/v1/volumes?" + v.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Accept", "application/json")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("googlebooks: http %d: %s", resp.StatusCode, string(b))
	}
	var r struct {
		Items []struct {
			VolumeInfo struct {
				Title         string   `json:"title"`
				Authors       []string `json:"authors"`
				Publisher     string   `json:"publisher"`
				PublishedDate string   `json:"publishedDate"`
				InfoLink      string   `json:"infoLink"`
			} `json:"volumeInfo"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return schema.Entry{}, err
	}
	if len(r.Items) == 0 {
		return schema.Entry{}, fmt.Errorf("googlebooks: no results")
	}
	vi := r.Items[0].VolumeInfo
	var e schema.Entry
	e.Type = "book"
	e.ID = schema.NewID()
	e.APA7.Title = strings.TrimSpace(vi.Title)
	if len(vi.Authors) > 0 {
		fam, giv := splitAuthor(vi.Authors[0])
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
		}
	}
	e.APA7.Publisher = strings.TrimSpace(vi.Publisher)
	if y := dates.ExtractYear(vi.PublishedDate); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.URL = strings.TrimSpace(vi.InfoLink)
	if e.APA7.URL != "" {
		e.APA7.Accessed = dates.NowISO()
	}
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from Google Books.", e.APA7.Title)
	e.Annotation.Keywords = []string{"book"}
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func searchCrossref(ctx context.Context, title, author string) (schema.Entry, error) {
	v := url.Values{}
	if strings.TrimSpace(title) != "" {
		v.Set("query.title", title)
	}
	if strings.TrimSpace(author) != "" {
		v.Set("query.author", author)
	}
	v.Set("rows", "1")
	v.Set("filter", "type:book")
	endpoint := "https://api.crossref.org/works?" + v.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Accept", "application/json")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("crossref: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Message struct {
			Items []struct {
				Title     []string                         `json:"title"`
				Author    []struct{ Given, Family string } `json:"author"`
				Publisher string                           `json:"publisher"`
				Issued    struct {
					DateParts [][]int `json:"date-parts"`
				} `json:"issued"`
				URL string `json:"URL"`
			} `json:"items"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Message.Items) == 0 {
		return schema.Entry{}, fmt.Errorf("crossref: no results")
	}
	it := out.Message.Items[0]
	var e schema.Entry
	e.Type = "book"
	e.ID = schema.NewID()
	if len(it.Title) > 0 {
		e.APA7.Title = strings.TrimSpace(it.Title[0])
	}
	if len(it.Author) > 0 {
		fam := strings.TrimSpace(it.Author[0].Family)
		giv := strings.TrimSpace(it.Author[0].Given)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: names.Initials(giv)})
		}
	}
	e.APA7.Publisher = strings.TrimSpace(it.Publisher)
	if y := yearFromDateParts(it.Issued.DateParts); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.URL = strings.TrimSpace(it.URL)
	if e.APA7.URL != "" {
		e.APA7.Accessed = dates.NowISO()
	}
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from Crossref.", e.APA7.Title)
	e.Annotation.Keywords = []string{"book"}
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// --- Crossref ---

func fetchCrossrefByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
	q := url.Values{}
	// Crossref supports filter=isbn:... for books/chapters
	q.Set("filter", "isbn:"+strings.TrimSpace(isbn))
	q.Set("rows", "1")
	endpoint := "https://api.crossref.org/works?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "application/json")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("crossref: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Message struct {
			Items []struct {
				Title          []string                         `json:"title"`
				Author         []struct{ Given, Family string } `json:"author"`
				Publisher      string                           `json:"publisher"`
				PublishedPrint struct {
					DateParts [][]int `json:"date-parts"`
				} `json:"published-print"`
				Issued struct {
					DateParts [][]int `json:"date-parts"`
				} `json:"issued"`
				DOI  string `json:"DOI"`
				URL  string `json:"URL"`
				Type string `json:"type"`
			} `json:"items"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Message.Items) == 0 {
		return schema.Entry{}, fmt.Errorf("crossref: no results")
	}
	it := out.Message.Items[0]
	var e schema.Entry
	e.Type = "book"
	if len(it.Title) > 0 {
		e.APA7.Title = strings.TrimSpace(it.Title[0])
	}
	e.APA7.Publisher = strings.TrimSpace(it.Publisher)
	e.APA7.DOI = strings.TrimSpace(it.DOI)
	e.APA7.URL = strings.TrimSpace(it.URL)
	e.APA7.ISBN = strings.TrimSpace(isbn)
	if strings.TrimSpace(e.APA7.URL) != "" {
		e.APA7.Accessed = dates.NowISO()
	}
	// choose date
	y := yearFromDateParts(it.PublishedPrint.DateParts)
	if y == 0 {
		y = yearFromDateParts(it.Issued.DateParts)
	}
	if y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	for _, a := range it.Author {
		fam := strings.TrimSpace(a.Family)
		giv := strings.TrimSpace(a.Given)
		if fam != "" {
			e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: names.Initials(giv)})
		}
	}
	if strings.TrimSpace(e.Annotation.Summary) == "" && e.APA7.Title != "" {
		if e.APA7.Publisher != "" && e.APA7.Year != nil {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s, %d) from Crossref.", e.APA7.Title, e.APA7.Publisher, *e.APA7.Year)
		} else if e.APA7.Publisher != "" {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s (%s) from Crossref.", e.APA7.Title, e.APA7.Publisher)
		} else {
			e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from Crossref.", e.APA7.Title)
		}
	}
	if len(e.Annotation.Keywords) == 0 {
		e.Annotation.Keywords = []string{"book"}
	}
	if strings.TrimSpace(e.ID) == "" {
		e.ID = schema.NewID()
	}
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func yearFromDateParts(dp [][]int) int {
	if len(dp) == 0 || len(dp[0]) == 0 {
		return 0
	}
	return dp[0][0]
}

// --- OCLC Classify (WorldCat) ---

func fetchOCLCClassifyByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
	q := url.Values{}
	q.Set("isbn", strings.TrimSpace(isbn))
	q.Set("summary", "true")
	endpoint := "http://classify.oclc.org/classify2/Classify?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "application/xml")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("oclc: http %d: %s", resp.StatusCode, string(b))
	}
	var doc struct {
		XMLName xml.Name `xml:"classify"`
		Works   struct {
			Work []struct {
				Title  string `xml:"title,attr"`
				Author string `xml:"author,attr"`
				Year   string `xml:"hyr,attr"`
			} `xml:"work"`
		} `xml:"works"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return schema.Entry{}, err
	}
	if len(doc.Works.Work) == 0 {
		return schema.Entry{}, fmt.Errorf("oclc: no works")
	}
	w := doc.Works.Work[0]
	var e schema.Entry
	e.Type = "book"
	e.APA7.Title = strings.TrimSpace(w.Title)
	fam, giv := splitAuthor(w.Author)
	if fam != "" {
		e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
	}
	e.APA7.ISBN = strings.TrimSpace(isbn)
	if y := dates.ExtractYear(strings.TrimSpace(w.Year)); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	if e.APA7.Title == "" {
		return schema.Entry{}, fmt.Errorf("oclc: empty title")
	}
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from OCLC Classify.", e.APA7.Title)
	e.Annotation.Keywords = []string{"book"}
	e.ID = schema.NewID()
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func splitAuthor(s string) (family, given string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	if i := strings.Index(s, ","); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// --- British National Bibliography (BNB) SPARQL ---

func fetchBNBByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
	// Public SPARQL endpoint
	endpoint := "https://bnb.data.bl.uk/sparql"
	// Try both raw and hyphenless forms
	norm := strings.ReplaceAll(strings.TrimSpace(isbn), " ", "")
	norm = strings.ReplaceAll(norm, "-", "")
	query := fmt.Sprintf(`PREFIX dcterms: <http://purl.org/dc/terms/>
PREFIX bibo: <http://purl.org/ontology/bibo/>
SELECT ?title ?publisherName ?date WHERE {
  {
    ?w bibo:isbn ?n .
    FILTER(REPLACE(STR(?n), "-", "") = "%s")
  } UNION {
    ?w bibo:isbn13 ?n13 .
    FILTER(REPLACE(STR(?n13), "-", "") = "%s")
  } UNION {
    ?w bibo:isbn10 ?n10 .
    FILTER(REPLACE(STR(?n10), "-", "") = "%s")
  }
  OPTIONAL { ?w dcterms:title ?title }
  OPTIONAL {
    ?w dcterms:publisher ?pub .
    OPTIONAL { ?pub <http://www.w3.org/2000/01/rdf-schema#label> ?publisherName }
  }
  OPTIONAL { ?w dcterms:issued ?date }
}
LIMIT 1`, norm, norm, norm)
	v := url.Values{}
	v.Set("query", query)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(v.Encode()))
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("bnb: http %d: %s", resp.StatusCode, string(b))
	}
	var sr struct {
		Results struct {
			Bindings []map[string]map[string]string `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return schema.Entry{}, err
	}
	if len(sr.Results.Bindings) == 0 {
		return schema.Entry{}, fmt.Errorf("bnb: no results")
	}
	b := sr.Results.Bindings[0]
	title := getSP(b, "title")
	publisher := getSP(b, "publisherName")
	date := getSP(b, "date")
	var e schema.Entry
	e.Type = "book"
	e.APA7.Title = title
	e.APA7.Publisher = publisher
	if y := dates.ExtractYear(date); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.ISBN = strings.TrimSpace(isbn)
	if e.APA7.Title == "" {
		return schema.Entry{}, fmt.Errorf("bnb: empty title")
	}
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from BNB.", e.APA7.Title)
	e.Annotation.Keywords = []string{"book"}
	e.ID = schema.NewID()
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

func getSP(m map[string]map[string]string, key string) string {
	if v, ok := m[key]; ok {
		return strings.TrimSpace(v["value"])
	}
	return ""
}

// --- openBD ---

func fetchOpenBDByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
	endpoint := "https://api.openbd.jp/v1/get?isbn=" + url.QueryEscape(strings.TrimSpace(isbn))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "application/json")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("openbd: http %d: %s", resp.StatusCode, string(b))
	}
	var arr []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		return schema.Entry{}, err
	}
	if len(arr) == 0 || arr[0] == nil {
		return schema.Entry{}, fmt.Errorf("openbd: no data")
	}
	m := arr[0]
	// Most useful at summary level
	var summary struct {
		Title     string `json:"title"`
		Publisher string `json:"publisher"`
		Pubdate   string `json:"pubdate"`
		Author    string `json:"author"`
	}
	if s, ok := m["summary"].(map[string]any); ok {
		buf, _ := json.Marshal(s)
		_ = json.Unmarshal(buf, &summary)
	}
	var e schema.Entry
	e.Type = "book"
	e.APA7.Title = strings.TrimSpace(summary.Title)
	e.APA7.Publisher = strings.TrimSpace(summary.Publisher)
	if y := dates.ExtractYear(summary.Pubdate); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.ISBN = strings.TrimSpace(isbn)
	// openBD author is a single string often "Family, Given" or katakana
	if fam, giv := splitAuthor(summary.Author); fam != "" {
		e.APA7.Authors = append(e.APA7.Authors, schema.Author{Family: fam, Given: giv})
	}
	if e.APA7.Title == "" {
		return schema.Entry{}, fmt.Errorf("openbd: empty title")
	}
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from openBD.", e.APA7.Title)
	e.Annotation.Keywords = []string{"book"}
	e.ID = schema.NewID()
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}

// --- Library of Congress ---

func fetchLoCByISBN(ctx context.Context, isbn string) (schema.Entry, error) {
	// LoC JSON API search by isbn
	endpoint := "https://www.loc.gov/search/?fo=json&q=" + url.QueryEscape("isbn:"+strings.TrimSpace(isbn))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return schema.Entry{}, err
	}
	req.Header.Set("Accept", "application/json")
	httpx.SetUA(req)
	resp, err := client.Do(req)
	if err != nil {
		return schema.Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return schema.Entry{}, fmt.Errorf("loc: http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Results []struct {
			Title string `json:"title"`
			Date  string `json:"date"`
			URL   string `json:"url"`
			// Publisher not always provided; can appear in extras
			// We keep mapping minimal and robust.
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return schema.Entry{}, err
	}
	if len(out.Results) == 0 {
		return schema.Entry{}, fmt.Errorf("loc: no results")
	}
	r := out.Results[0]
	var e schema.Entry
	e.Type = "book"
	e.APA7.Title = strings.TrimSpace(r.Title)
	if y := dates.ExtractYear(strings.TrimSpace(r.Date)); y > 0 {
		y2 := y
		e.APA7.Year = &y2
	}
	e.APA7.URL = strings.TrimSpace(r.URL)
	if e.APA7.URL != "" {
		e.APA7.Accessed = dates.NowISO()
	}
	e.APA7.ISBN = strings.TrimSpace(isbn)
	if e.APA7.Title == "" {
		return schema.Entry{}, fmt.Errorf("loc: empty title")
	}
	e.Annotation.Summary = fmt.Sprintf("Bibliographic record for %s from Library of Congress.", e.APA7.Title)
	e.Annotation.Keywords = []string{"book"}
	e.ID = schema.NewID()
	sanitize.CleanEntry(&e)
	if err := e.Validate(); err != nil {
		return schema.Entry{}, err
	}
	return e, nil
}
