package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"bibliography/src/internal/schema"
)

// RebuildBibLibrary regenerates the consolidated BibTeX library from all YAML entries.
// This keeps the BibTeX view as a faithful mirror of the authoritative YAML source
// while we transition storage to BibTeX.
func RebuildBibLibrary() error {
	entries, err := ReadAll()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(BibFile), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	// Deterministic order: by type then title then id
	sort.Slice(entries, func(i, j int) bool {
		ei, ej := entries[i], entries[j]
		if ei.Type != ej.Type {
			return ei.Type < ej.Type
		}
		ti := strings.ToLower(strings.TrimSpace(ei.APA7.Title))
		tj := strings.ToLower(strings.TrimSpace(ej.APA7.Title))
		if ti != tj {
			return ti < tj
		}
		return ei.ID < ej.ID
	})
	for _, e := range entries {
		buf.WriteString(entryToBibTeX(e))
		if !strings.HasSuffix(buf.String(), "\n\n") {
			buf.WriteString("\n")
		}
	}
	return os.WriteFile(BibFile, buf.Bytes(), 0o644)
}

// ExportYAMLToBib reads all YAML entries from data/citations and writes a consolidated
// BibTeX file to target. This is intended for one-time migrations.
func ExportYAMLToBib(target string) error {
	entries, err := readAllYAML()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	sort.Slice(entries, func(i, j int) bool {
		ei, ej := entries[i], entries[j]
		if ei.Type != ej.Type {
			return ei.Type < ej.Type
		}
		ti := strings.ToLower(strings.TrimSpace(ei.APA7.Title))
		tj := strings.ToLower(strings.TrimSpace(ej.APA7.Title))
		if ti != tj {
			return ti < tj
		}
		return ei.ID < ej.ID
	})
	for _, e := range entries {
		buf.WriteString(entryToBibTeX(e))
	}
	return os.WriteFile(target, buf.Bytes(), 0o644)
}

// entryToBibTeX converts a schema.Entry into a BibTeX record string.
// We use non-standard fields 'abstract' and 'keywords' to capture annotations.
func entryToBibTeX(e schema.Entry) string {
	typ := bibTypeFor(e.Type)
	key := bibKeyFor(e)
	// field helpers
	w := func(k, v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		return fmt.Sprintf("  %s = {%s},\n", k, escapeBib(v))
	}
	// Authors
	authors := formatAuthors(e.APA7.Authors)
	year := ""
	if e.APA7.Year != nil {
		year = fmt.Sprintf("%d", *e.APA7.Year)
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "@%s{%s,\n", typ, key)
	if authors != "" {
		b.WriteString(w("author", authors))
	}
	b.WriteString(w("title", e.APA7.Title))
	switch strings.ToLower(strings.TrimSpace(e.Type)) {
	case "article":
		b.WriteString(w("journal", coalesce(e.APA7.Journal, e.APA7.ContainerTitle)))
		b.WriteString(w("volume", e.APA7.Volume))
		b.WriteString(w("number", e.APA7.Issue))
		b.WriteString(w("pages", e.APA7.Pages))
		b.WriteString(w("doi", e.APA7.DOI))
		b.WriteString(w("url", e.APA7.URL))
	case "book":
		b.WriteString(w("publisher", e.APA7.Publisher))
		b.WriteString(w("address", e.APA7.PublisherLocation))
		b.WriteString(w("edition", e.APA7.Edition))
		b.WriteString(w("isbn", e.APA7.ISBN))
		b.WriteString(w("doi", e.APA7.DOI))
		b.WriteString(w("url", e.APA7.URL))
	case "patent":
		// Map to @misc; include publisher/assignee and url
		b.WriteString(w("howpublished", e.APA7.Publisher))
		b.WriteString(w("url", e.APA7.URL))
	case "website":
		b.WriteString(w("howpublished", coalesce(e.APA7.Publisher, "Website")))
		b.WriteString(w("url", e.APA7.URL))
		if strings.TrimSpace(e.APA7.Accessed) != "" {
			b.WriteString(w("note", "Accessed: "+e.APA7.Accessed))
		}
	case "movie", "video", "song", "rfc", "report", "dataset", "software":
		// Generic mapping; try to set container/publisher/url
		b.WriteString(w("howpublished", coalesce(e.APA7.Publisher, e.APA7.ContainerTitle)))
		b.WriteString(w("url", e.APA7.URL))
		b.WriteString(w("doi", e.APA7.DOI))
	default:
		b.WriteString(w("url", e.APA7.URL))
		b.WriteString(w("doi", e.APA7.DOI))
	}
	b.WriteString(w("year", year))
	if strings.TrimSpace(e.APA7.Date) != "" {
		b.WriteString(w("date", e.APA7.Date))
	}
	// Non-standard but widely supported
	b.WriteString(w("abstract", e.Annotation.Summary))
	if len(e.Annotation.Keywords) > 0 {
		b.WriteString(w("keywords", strings.Join(e.Annotation.Keywords, ", ")))
	}
	// Always include our UUID and original type for traceability/round-trip
	b.WriteString(w("_id", e.ID))
	b.WriteString(w("_type", e.Type))
	// Close record; remove trailing comma if present
	out := b.String()
	out = strings.TrimRight(out, "\n")
	out = strings.TrimRight(out, ",")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += "}\n\n"
	return out
}

func escapeBib(s string) string {
	// Minimal escaping; preserve LaTeX-friendly characters as-is
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "{", "\\{")
	s = strings.ReplaceAll(s, "}", "\\}")
	return strings.TrimSpace(s)
}

func coalesce(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func formatAuthors(as schema.Authors) string {
	if len(as) == 0 {
		return ""
	}
	parts := make([]string, 0, len(as))
	for _, a := range as {
		fam := strings.TrimSpace(a.Family)
		giv := strings.TrimSpace(a.Given)
		if fam == "" && giv == "" {
			continue
		}
		if fam == "" {
			parts = append(parts, giv)
			continue
		}
		if giv == "" {
			parts = append(parts, fam)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s, %s", fam, giv))
	}
	return strings.Join(parts, " and ")
}

func bibTypeFor(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "article":
		return "article"
	case "book":
		return "book"
	default:
		return "misc"
	}
}

func bibKeyFor(e schema.Entry) string {
	// Prefer UUID without dashes to ensure uniqueness and BibTeX-compatibility
	k := strings.ReplaceAll(strings.ToLower(e.ID), "-", "")
	if k == "" {
		k = strings.ToLower(strings.ReplaceAll(schema.Slugify(e.APA7.Title, e.APA7.Year), "-", ""))
		if k == "" {
			k = "entry"
		}
	}
	return k
}

// --- BibTeX parsing/upsert ---

type bibRecord struct {
	typ    string
	key    string
	fields map[string]string
}

// UpdateBibEntry inserts or replaces the entry with the same _id in BibFile.
func UpdateBibEntry(e schema.Entry) error {
	var records []bibRecord
	if b, err := os.ReadFile(BibFile); err == nil && len(b) > 0 {
		rs, perr := parseBib(string(b))
		if perr != nil {
			return perr
		}
		records = rs
	}
	// Convert to record
	rec := entryToRecord(e)
	// Replace by _id match else append
	id := strings.ToLower(strings.TrimSpace(e.ID))
	found := false
	for i := range records {
		if strings.ToLower(records[i].fields["_id"]) == id && id != "" {
			records[i] = rec
			found = true
			break
		}
	}
	if !found {
		records = append(records, rec)
	}
	// Deterministic order
	sort.Slice(records, func(i, j int) bool {
		if records[i].typ != records[j].typ {
			return records[i].typ < records[j].typ
		}
		if records[i].fields["title"] != records[j].fields["title"] {
			return strings.ToLower(records[i].fields["title"]) < strings.ToLower(records[j].fields["title"])
		}
		return records[i].key < records[j].key
	})
	// Write back
	var buf bytes.Buffer
	for _, r := range records {
		buf.WriteString(renderRecord(r))
	}
	if err := os.MkdirAll(filepath.Dir(BibFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(BibFile, buf.Bytes(), 0o644)
}

func entryToRecord(e schema.Entry) bibRecord {
	// Build from the same mapping used by entryToBibTeX
	// We render via renderRecord to keep formatting in one place.
	// Minimal map to ease deterministic ordering later.
	m := map[string]string{}
	if len(e.APA7.Authors) > 0 {
		m["author"] = formatAuthors(e.APA7.Authors)
	}
	m["title"] = e.APA7.Title
	switch strings.ToLower(strings.TrimSpace(e.Type)) {
	case "article":
		if v := coalesce(e.APA7.Journal, e.APA7.ContainerTitle); v != "" {
			m["journal"] = v
		}
		if v := e.APA7.Volume; v != "" {
			m["volume"] = v
		}
		if v := e.APA7.Issue; v != "" {
			m["number"] = v
		}
		if v := e.APA7.Pages; v != "" {
			m["pages"] = v
		}
		if v := e.APA7.DOI; v != "" {
			m["doi"] = v
		}
		if v := e.APA7.URL; v != "" {
			m["url"] = v
		}
	case "book":
		if v := e.APA7.Publisher; v != "" {
			m["publisher"] = v
		}
		if v := e.APA7.PublisherLocation; v != "" {
			m["address"] = v
		}
		if v := e.APA7.Edition; v != "" {
			m["edition"] = v
		}
		if v := e.APA7.ISBN; v != "" {
			m["isbn"] = v
		}
		if v := e.APA7.DOI; v != "" {
			m["doi"] = v
		}
		if v := e.APA7.URL; v != "" {
			m["url"] = v
		}
	default:
		if v := coalesce(e.APA7.Publisher, e.APA7.ContainerTitle); v != "" {
			m["howpublished"] = v
		}
		if v := e.APA7.URL; v != "" {
			m["url"] = v
		}
		if v := e.APA7.DOI; v != "" {
			m["doi"] = v
		}
	}
	if e.APA7.Year != nil {
		m["year"] = fmt.Sprintf("%d", *e.APA7.Year)
	}
	if strings.TrimSpace(e.APA7.Date) != "" {
		m["date"] = e.APA7.Date
	}
	if v := e.Annotation.Summary; strings.TrimSpace(v) != "" {
		m["abstract"] = v
	}
	if len(e.Annotation.Keywords) > 0 {
		m["keywords"] = strings.Join(e.Annotation.Keywords, ", ")
	}
	m["_id"] = e.ID
	m["_type"] = e.Type
	return bibRecord{typ: bibTypeFor(e.Type), key: bibKeyFor(e), fields: m}
}

func renderRecord(r bibRecord) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "@%s{%s,\n", r.typ, r.key)
	// stable field order: author, title, journal/howpublished/publisher..., then remaining sorted
	order := []string{"author", "title", "journal", "booktitle", "howpublished", "publisher", "address", "edition", "volume", "number", "pages", "year", "doi", "isbn", "url", "abstract", "keywords", "_id", "_type"}
	seen := map[string]bool{}
	for _, k := range order {
		if v, ok := r.fields[k]; ok && strings.TrimSpace(v) != "" {
			b.WriteString(fmt.Sprintf("  %s = {%s},\n", k, escapeBib(v)))
			seen[k] = true
		}
	}
	// any extras
	extras := make([]string, 0, len(r.fields))
	for k := range r.fields {
		if !seen[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	for _, k := range extras {
		v := r.fields[k]
		if strings.TrimSpace(v) != "" {
			b.WriteString(fmt.Sprintf("  %s = {%s},\n", k, escapeBib(v)))
		}
	}
	out := b.String()
	out = strings.TrimRight(out, "\n")
	out = strings.TrimRight(out, ",")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += "}\n\n"
	return out
}

func parseBib(s string) ([]bibRecord, error) {
	i := 0
	n := len(s)
	var recs []bibRecord
	skipWS := func() {
		for i < n {
			if s[i] == '%' {
				for i < n && s[i] != '\n' {
					i++
				}
				continue
			}
			if strings.IndexByte(" \t\r\n", s[i]) >= 0 {
				i++
			} else {
				break
			}
		}
	}
	readIdent := func() string {
		start := i
		for i < n && (('a' <= s[i] && s[i] <= 'z') || ('A' <= s[i] && s[i] <= 'Z')) {
			i++
		}
		return s[start:i]
	}
	for {
		skipWS()
		if i >= n {
			break
		}
		if s[i] != '@' {
			i++
			continue
		}
		i++
		skipWS()
		typ := strings.ToLower(readIdent())
		skipWS()
		if i >= n || (s[i] != '{' && s[i] != '(') {
			return nil, fmt.Errorf("invalid bib: expected '{' after type")
		}
		// use '{'
		// advance past delimiter
		i++
		skipWS()
		// key up to comma
		start := i
		for i < n && s[i] != ',' {
			i++
		}
		if i >= n {
			return nil, fmt.Errorf("invalid bib: missing comma after key")
		}
		key := strings.TrimSpace(s[start:i])
		i++ // skip comma
		fields := map[string]string{}
		for {
			skipWS()
			if i >= n {
				return nil, fmt.Errorf("invalid bib: unexpected EOF in fields")
			}
			if s[i] == '}' || s[i] == ')' {
				i++
				break
			}
			// field name
			fstart := i
			for i < n && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') || s[i] == '_') {
				i++
			}
			fname := strings.ToLower(strings.TrimSpace(s[fstart:i]))
			skipWS()
			if i >= n || s[i] != '=' {
				return nil, fmt.Errorf("invalid bib: expected '=' after field name")
			}
			i++
			skipWS()
			// value: brace-delimited or quote-delimited
			val := ""
			if i < n && s[i] == '{' {
				depth := 0
				i++
				vstart := i
				for i < n {
					if s[i] == '\\' {
						i += 2
						continue
					}
					if s[i] == '{' {
						depth++
						i++
						continue
					}
					if s[i] == '}' {
						if depth == 0 {
							val = s[vstart:i]
							i++
							break
						}
						depth--
						i++
						continue
					}
					i++
				}
			} else if i < n && s[i] == '"' {
				i++
				vstart := i
				for i < n {
					if s[i] == '\\' {
						i += 2
						continue
					}
					if s[i] == '"' {
						val = s[vstart:i]
						i++
						break
					}
					i++
				}
			} else {
				// bare value until comma/brace
				vstart := i
				for i < n && s[i] != ',' && s[i] != '}' && s[i] != ')' {
					i++
				}
				val = strings.TrimSpace(s[vstart:i])
			}
			fields[fname] = unescapeBib(val)
			skipWS()
			if i < n && s[i] == ',' {
				i++
				continue
			}
			if i < n && (s[i] == '}' || s[i] == ')') {
				i++
				break
			}
		}
		recs = append(recs, bibRecord{typ: typ, key: key, fields: fields})
	}
	return recs, nil
}

func unescapeBib(s string) string {
	s = strings.ReplaceAll(s, "\\{", "{")
	s = strings.ReplaceAll(s, "\\}", "}")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// bibToEntries converts bib records into schema entries.
func bibToEntries(rs []bibRecord) []schema.Entry {
	out := make([]schema.Entry, 0, len(rs))
	for _, r := range rs {
		e := schema.Entry{}
		if id := strings.TrimSpace(r.fields["_id"]); id != "" {
			e.ID = id
		} else {
			e.ID = schema.NewID()
		}
		t := strings.TrimSpace(r.fields["_type"])
		if t == "" {
			// derive from bib type as best effort
			switch r.typ {
			case "article":
				t = "article"
			case "book":
				t = "book"
			default:
				t = "website"
			}
		}
		e.Type = t
		// Authors
		if a := strings.TrimSpace(r.fields["author"]); a != "" {
			e.APA7.Authors = parseAuthorsField(a)
		}
		e.APA7.Title = r.fields["title"]
		e.APA7.Journal = r.fields["journal"]
		if e.APA7.Journal == "" {
			e.APA7.ContainerTitle = r.fields["booktitle"]
		}
		e.APA7.Volume = r.fields["volume"]
		e.APA7.Issue = r.fields["number"]
		e.APA7.Pages = r.fields["pages"]
		e.APA7.DOI = r.fields["doi"]
		e.APA7.ISBN = r.fields["isbn"]
		e.APA7.URL = r.fields["url"]
		e.APA7.Publisher = r.fields["publisher"]
		e.APA7.PublisherLocation = r.fields["address"]
		e.APA7.Edition = r.fields["edition"]
		if y := strings.TrimSpace(r.fields["year"]); y != "" {
			var yy int
			fmt.Sscanf(y, "%d", &yy)
			if yy > 0 {
				e.APA7.Year = &yy
			}
		}
		if d := strings.TrimSpace(r.fields["date"]); d != "" {
			e.APA7.Date = d
		}
		e.Annotation.Summary = r.fields["abstract"]
		if kw := strings.TrimSpace(r.fields["keywords"]); kw != "" {
			e.Annotation.Keywords = splitKeywords(kw)
		}
		out = append(out, e)
	}
	return out
}

func parseAuthorsField(s string) schema.Authors {
	// Split on ' and ' outside braces (we don't emit braces), simple split works
	parts := strings.Split(s, " and ")
	out := make([]schema.Author, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Prefer "Family, Given"
		if i := strings.Index(p, ","); i >= 0 {
			fam := strings.TrimSpace(p[:i])
			giv := strings.TrimSpace(p[i+1:])
			out = append(out, schema.Author{Family: fam, Given: giv})
		} else {
			out = append(out, schema.Author{Family: p})
		}
	}
	return schema.Authors(out)
}

func splitKeywords(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
