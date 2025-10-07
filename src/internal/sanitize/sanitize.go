package sanitize

import (
    "net/url"
    "strings"

    "bibliography/src/internal/schema"
)

// CleanString trims and removes ASCII control characters except tab/newline/carriage
// return up to max runes (if max <= 0, no truncation).
func CleanString(s string, max int) string {
    s = strings.TrimSpace(s)
    if s == "" {
        return s
    }
    // remove controls except \n, \t, \r
    var b strings.Builder
    for _, r := range s {
        if r == '\n' || r == '\t' || r == '\r' || (r >= 0x20 && r != 0x7f) {
            b.WriteRune(r)
            if max > 0 && b.Len() >= max {
                break
            }
        }
    }
    return strings.TrimSpace(b.String())
}

// CleanURL returns a validated http/https URL or empty string.
func CleanURL(raw string) string {
    raw = strings.TrimSpace(raw)
    if raw == "" {
        return ""
    }
    u, err := url.Parse(raw)
    if err != nil || u.Scheme == "" || u.Host == "" {
        return ""
    }
    if u.Scheme != "http" && u.Scheme != "https" {
        return ""
    }
    // remove embedded whitespace
    u.Path = strings.ReplaceAll(u.Path, " ", "%20")
    return u.String()
}

// CleanKeywords trims, lowercases, dedupes, and limits keyword count.
func CleanKeywords(keys []string) []string {
    if len(keys) == 0 {
        return nil
    }
    const maxKeywords = 64
    const maxLen = 64
    seen := map[string]bool{}
    out := make([]string, 0, len(keys))
    for _, k := range keys {
        k = strings.ToLower(CleanString(k, maxLen))
        if k == "" || seen[k] {
            continue
        }
        seen[k] = true
        out = append(out, k)
        if len(out) >= maxKeywords {
            break
        }
    }
    if len(out) == 0 {
        return nil
    }
    return out
}

// CleanAuthors sanitizes author names.
func CleanAuthors(authors schema.Authors) schema.Authors {
    if len(authors) == 0 {
        return nil
    }
    const max = 256
    out := make([]schema.Author, 0, len(authors))
    for _, a := range authors {
        fam := CleanString(a.Family, max)
        giv := CleanString(a.Given, max)
        if fam == "" && giv == "" {
            continue
        }
        out = append(out, schema.Author{Family: fam, Given: giv})
    }
    if len(out) == 0 {
        return nil
    }
    return out
}

// CleanEntry applies conservative sanitization to all strings in the entry.
func CleanEntry(e *schema.Entry) {
    if e == nil { return }
    e.ID = CleanString(e.ID, 64)
    e.Type = CleanString(e.Type, 32)
    // APA7 fields
    e.APA7.Title = CleanString(e.APA7.Title, 512)
    e.APA7.ContainerTitle = CleanString(e.APA7.ContainerTitle, 512)
    e.APA7.Edition = CleanString(e.APA7.Edition, 128)
    e.APA7.Publisher = CleanString(e.APA7.Publisher, 512)
    e.APA7.PublisherLocation = CleanString(e.APA7.PublisherLocation, 256)
    e.APA7.Journal = CleanString(e.APA7.Journal, 512)
    e.APA7.Volume = CleanString(e.APA7.Volume, 64)
    e.APA7.Issue = CleanString(e.APA7.Issue, 64)
    e.APA7.Pages = CleanString(e.APA7.Pages, 64)
    e.APA7.DOI = CleanString(e.APA7.DOI, 128)
    e.APA7.ISBN = CleanString(e.APA7.ISBN, 64)
    e.APA7.URL = CleanURL(e.APA7.URL)
    e.APA7.BibTeXURL = CleanURL(e.APA7.BibTeXURL)
    e.APA7.Accessed = CleanString(e.APA7.Accessed, 32)
    e.APA7.Date = CleanString(e.APA7.Date, 32)
    // Authors and annotations
    e.APA7.Authors = CleanAuthors(e.APA7.Authors)
    e.Annotation.Summary = CleanString(e.Annotation.Summary, 12000)
    e.Annotation.Keywords = CleanKeywords(e.Annotation.Keywords)
}

