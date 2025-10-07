package webfetch

import (
    "context"
    "io"
    "net/http"
    "strings"
    "testing"
    datespkg "bibliography/src/internal/dates"
    namespkg "bibliography/src/internal/names"
)

type fakeHTTP struct {
	status  int
	body    string
	headers map[string]string
}

func (f fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	h := make(http.Header)
	for k, v := range f.headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(f.body)), Header: h}, nil
}

func TestFetchArticleByURL_HTMLPath(t *testing.T) {
	html := `<!doctype html><html><head>
    <meta property="og:title" content="Sample Title">
    <meta property="og:site_name" content="Example Site">
    <meta name="description" content="A description.">
    <script type="application/ld+json">{"@type":"NewsArticle","author":{"name":"Jane Doe"},"headline":"Sample Title","datePublished":"2024-06-01","publisher":{"name":"Example Site"}}</script>
    </head><body>...</body></html>`
	old := client
	defer func() { client = old }()
	client = fakeHTTP{status: 200, body: html, headers: map[string]string{"Content-Type": "text/html"}}
	e, err := FetchArticleByURL(context.Background(), "https://news.example.com/post")
	if err != nil {
		t.Fatalf("FetchArticleByURL: %v", err)
	}
	if e.APA7.Title != "Sample Title" || e.APA7.ContainerTitle != "Example Site" {
		t.Fatalf("bad mapping: %+v", e)
	}
	if len(e.APA7.Authors) == 0 || e.APA7.Authors[0].Family != "Doe" {
		t.Fatalf("authors not parsed: %+v", e.APA7.Authors)
	}
	if e.APA7.Accessed == "" || e.APA7.URL == "" {
		t.Fatalf("url/accessed missing")
	}
}

func TestFetchArticleByURL_PDFPath(t *testing.T) {
	pdf := "%PDF-1.4\n1 0 obj\n<< /Title (Sometitle) /Author (Doe, Jane) /CreationDate (D:20230601120000Z) >>\nendobj\n"
	old := client
	defer func() { client = old }()
	client = fakeHTTP{status: 200, body: pdf, headers: map[string]string{"Content-Type": "application/pdf"}}
	e, err := FetchArticleByURL(context.Background(), "https://example.com/x.pdf")
	if err != nil {
		t.Fatalf("FetchArticleByURL pdf: %v", err)
	}
	if e.APA7.Title == "" || e.APA7.ContainerTitle == "" {
		t.Fatalf("expected title/container from PDF metadata")
	}
	if len(e.APA7.Authors) == 0 {
		t.Fatalf("expected authors")
	}
}

func TestFetchArticleByURL_Errors(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = fakeHTTP{status: 404, body: "not found"}
	if _, err := FetchArticleByURL(context.Background(), "https://example.com/x"); err == nil {
		t.Fatalf("expected http error")
	}
	if _, err := FetchArticleByURL(context.Background(), ":://bad"); err == nil {
		t.Fatalf("expected invalid url error")
	}
}

func TestHelpers(t *testing.T) {
	if h := hostOf("https://www.Example.com/path"); h != "example.com" {
		t.Fatalf("hostOf: %q", h)
	}
    fam, giv := namespkg.Split("Doe, Jane Q")
	if fam != "Doe" || giv != "J. Q." {
		t.Fatalf("splitName: %s %s", fam, giv)
	}
    fam, giv = namespkg.Split("Jane Q Public")
	if fam != "Public" || giv == "" {
		t.Fatalf("splitName parts: %s %s", fam, giv)
	}
    if y := datespkg.ExtractYear("May 2020"); y != 2020 {
        t.Fatalf("extractYear: %d", y)
    }
}

func TestParsers_OpenGraphAndJSONLD(t *testing.T) {
	body := `<!doctype html><html><head>
    <meta name="author" content="Acme Corp">
    <meta name="description" content="Desc here">
    <meta property="og:title" content="OG Title">
    </head><body>
    <script type="application/ld+json">[{"@type":"Article","headline":"LD Headline","author":[{"name":"John Smith"}],"publisher":"Pub","datePublished":"2023-01-01"}]</script>
    </body></html>`
	og, title := parseOpenGraphAndTitle(body)
	if og["og:description"] != "Desc here" || og["author"] != "Acme Corp" {
		t.Fatalf("og/meta mix: %+v", og)
	}
	if title != "" {
		t.Fatalf("title should be empty due to no <title>")
	}
	ld := parseJSONLDArticle(body)
	if ld.headline != "LD Headline" || len(ld.authors) == 0 || ld.publisher != "Pub" || ld.datePublished != "2023-01-01" {
		t.Fatalf("ld: %+v", ld)
	}
}

func TestMetaName(t *testing.T) {
	body := `<meta name="description" content="Hello"><meta name="author" content="Alice">`
	if metaName(body, "description") != "Hello" || metaName(body, "author") != "Alice" {
		t.Fatalf("metaName parse failed")
	}
}

func TestPDFHelpers(t *testing.T) {
	if pdfUnescape(`\(Hello\)`) != "(Hello)" {
		t.Fatalf("pdfUnescape")
	}
	if htmlUnescape("&amp; &lt; &gt;") != "& < >" {
		t.Fatalf("htmlUnescape")
	}
}
