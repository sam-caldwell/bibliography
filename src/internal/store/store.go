package store

import (
    "encoding/json"
    "errors"
    "fmt"
    "io/fs"
    "net/url"
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strings"
    // time removed; use dates.NowISO

    "gopkg.in/yaml.v3"

    "bibliography/src/internal/schema"
    "bibliography/src/internal/dates"
)

const (
    CitationsDir = "data/citations"
    MetadataDir  = "data/metadata"
    KeywordsJSON = "data/metadata/keywords.json"
    AuthorsJSON  = "data/metadata/authors.json"
    TitlesJSON   = "data/metadata/titles.json"
    ISBNJSON     = "data/metadata/isbn.json"
    DOIJSON      = "data/metadata/doi.json"
)

// --- Small helpers to lower duplication and cognitive load ---

// ensureMetaDir creates the metadata directory if missing.
func ensureMetaDir() error { return os.MkdirAll(MetadataDir, 0o755) }

// entryPath returns the repo-relative path to the YAML file for an entry id/type.
func entryPath(e schema.Entry) string {
    seg := dirForType(e.Type)
    return filepath.ToSlash(filepath.Join(CitationsDir, seg, e.ID+".yaml"))
}

// writeJSON writes the given value to the target JSON file with indentation.
func writeJSON(target string, v any) (string, error) {
    b, err := json.MarshalIndent(v, "", "  ")
    if err != nil { return "", err }
    if err := os.WriteFile(target, b, 0o644); err != nil { return "", err }
    return target, nil
}

// dirForType maps an entry type to its subdirectory under data/citations.
// Unknown types fall back to "citation"; some types use plural or aliased forms.
func dirForType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "article":
		return "article"
	case "movie":
		return "movie"
	case "video":
		return "video"
	case "patent":
		return "patent"
	case "song":
		return "song"
	case "book":
		return "books"
	case "website":
		return "site"
	case "rfc":
		return "rfc"
	default:
		return "citation"
	}
}

// SegmentForType returns the directory segment used for a given type (e.g., book->books).
func SegmentForType(typ string) string { return dirForType(typ) }

// WriteEntry validates and writes the entry YAML to data/citations/<segment>/<id>.yaml.
func WriteEntry(e schema.Entry) (string, error) {
	if strings.TrimSpace(e.ID) == "" {
		e.ID = schema.NewID()
	}
	if err := e.Validate(); err != nil {
		return "", err
	}
	subdir := filepath.Join(CitationsDir, dirForType(e.Type))
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(subdir, fmt.Sprintf("%s.yaml", e.ID))
	buf, err := yaml.Marshal(e)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ReadAll loads, validates, and returns all entries under data/citations.
func ReadAll() ([]schema.Entry, error) {
	var entries []schema.Entry
	if _, err := os.Stat(CitationsDir); errors.Is(err, fs.ErrNotExist) {
		return entries, nil
	}
	err := filepath.WalkDir(CitationsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var e schema.Entry
		if err := yaml.Unmarshal(data, &e); err != nil {
			return fmt.Errorf("invalid YAML in %s: %w", path, err)
		}
		if err := e.Validate(); err != nil {
			return fmt.Errorf("invalid entry in %s: %w", path, err)
		}
		entries = append(entries, e)
		return nil
	})
	return entries, err
}

// BuildKeywordIndex writes data/metadata/keywords.json mapping keyword -> list of entry YAML paths.
func BuildKeywordIndex(entries []schema.Entry) (string, error) {
    if err := ensureMetaDir(); err != nil {
        return "", err
    }
    index := map[string][]string{}
    for _, e := range entries {
        seen := map[string]bool{}

		// helper to add a token (lowercased, trimmed) to the index once per entry
        add := func(tok string) {
            t := strings.ToLower(strings.TrimSpace(tok))
            if t == "" || seen[t] {
                return
            }
            seen[t] = true
            index[t] = append(index[t], entryPath(e))
        }

		// 1) annotation keywords
		for _, k := range e.Annotation.Keywords {
			add(k)
		}

		// 2) words in the summary (generate keywords from summaries)
		for _, w := range tokenizeWords(e.Annotation.Summary) {
			add(w)
		}

		// 3) words in the title (and implicitly names within title)
		for _, w := range tokenizeWords(e.APA7.Title) {
			add(w)
		}

		// 4) publisher (full phrase and tokens)
		if strings.TrimSpace(e.APA7.Publisher) != "" {
			add(e.APA7.Publisher)
			for _, w := range tokenizeWords(e.APA7.Publisher) {
				add(w)
			}
		}

		// 5) publication container/journal (full phrases and tokens)
		if strings.TrimSpace(e.APA7.Journal) != "" {
			add(e.APA7.Journal)
			for _, w := range tokenizeWords(e.APA7.Journal) {
				add(w)
			}
		}
		if strings.TrimSpace(e.APA7.ContainerTitle) != "" {
			add(e.APA7.ContainerTitle)
			for _, w := range tokenizeWords(e.APA7.ContainerTitle) {
				add(w)
			}
		}

		// 6) year published
		if e.APA7.Year != nil {
			add(fmt.Sprintf("%d", *e.APA7.Year))
		}

		// 7) website domain (host and host without leading www.)
		if u := strings.TrimSpace(e.APA7.URL); u != "" {
			if parsed, err := url.Parse(u); err == nil {
				host := strings.ToLower(strings.TrimSpace(parsed.Host))
				if host != "" {
					add(host)
					add(strings.TrimPrefix(host, "www."))
				}
			}
		}

		// 8) type (e.g., article, book, website)
		if strings.TrimSpace(e.Type) != "" {
			add(e.Type)
		}
	}
	// sort lists for determinism
	for k := range index {
		sort.Strings(index[k])
	}
    return writeJSON(KeywordsJSON, index)
}

// BuildAuthorIndex writes data/metadata/authors.json mapping author name -> entry YAML paths.
// Author key format is "Family, Given" when both present; otherwise the non-empty name.
func BuildAuthorIndex(entries []schema.Entry) (string, error) {
    if err := ensureMetaDir(); err != nil {
        return "", err
    }
    index := map[string][]string{}
    for _, e := range entries {
        path := entryPath(e)
        // Deduplicate per author per entry
        perEntrySeen := map[string]bool{}
        for _, au := range e.APA7.Authors {
            name := strings.TrimSpace(au.Family)
            g := strings.TrimSpace(au.Given)
			if name == "" && g != "" {
				name = g
			} else if name != "" && g != "" {
				name = name + ", " + g
			}
			if name == "" || perEntrySeen[name] {
				continue
			}
			perEntrySeen[name] = true
			index[name] = append(index[name], path)
		}
	}
	// Sort lists for determinism
	for k := range index {
		sort.Strings(index[k])
	}
    return writeJSON(AuthorsJSON, index)
}

// BuildTitleIndex writes data/metadata/titles.json mapping entry YAML path -> tokenized title words.
func BuildTitleIndex(entries []schema.Entry) (string, error) {
    if err := ensureMetaDir(); err != nil {
        return "", err
    }
    index := map[string][]string{}
    for _, e := range entries {
        index[entryPath(e)] = tokenizeWords(e.APA7.Title)
    }
    return writeJSON(TitlesJSON, index)
}

// BuildISBNIndex writes data/metadata/isbn.json mapping entry YAML path -> ISBN for books with ISBNs.
func BuildISBNIndex(entries []schema.Entry) (string, error) {
    if err := ensureMetaDir(); err != nil {
        return "", err
    }
    index := map[string]string{}
    for _, e := range entries {
        if strings.ToLower(strings.TrimSpace(e.Type)) != "book" {
            continue
        }
        isbn := strings.TrimSpace(e.APA7.ISBN)
        if isbn == "" {
            continue
        }
        index[entryPath(e)] = isbn
    }
    return writeJSON(ISBNJSON, index)
}

// BuildDOIIndex writes data/metadata/doi.json mapping entry YAML path -> DOI for entries with DOIs.
func BuildDOIIndex(entries []schema.Entry) (string, error) {
    if err := ensureMetaDir(); err != nil {
        return "", err
    }
    index := map[string]string{}
    for _, e := range entries {
        doi := strings.TrimSpace(e.APA7.DOI)
        if doi == "" {
            continue
        }
        index[entryPath(e)] = doi
    }
    return writeJSON(DOIJSON, index)
}

var nonWord = regexp.MustCompile(`[^a-zA-Z0-9]+`)
var doiRegex = regexp.MustCompile(`(?i)10\.\d{4,9}/[-._;()/:A-Z0-9]+`)

// tokenizeWords splits a phrase into lowercased word tokens, filtering empties and 1-character tokens.
func tokenizeWords(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := nonWord.Split(s, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if len(p) >= 2 {
			out = append(out, p)
		}
	}
	return out
}

// ExtractDOI extracts a DOI-like token from an arbitrary string containing a DOI or DOI URL.
func ExtractDOI(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	match := doiRegex.FindString(s)
	if match == "" {
		return ""
	}
	// Normalize to lowercase except DOI can contain upper; preserve as found
	return strings.TrimSpace(match)
}

// NormalizeArticleDOI ensures an article's DOI and URL are consistent (doi and https://doi.org/<doi>). Returns true if modified.
func NormalizeArticleDOI(e *schema.Entry) bool {
	if e == nil {
		return false
	}
	if strings.ToLower(strings.TrimSpace(e.Type)) != "article" {
		return false
	}
	changed := false
	doi := strings.TrimSpace(e.APA7.DOI)
	if doi == "" {
		if d := ExtractDOI(e.APA7.URL); d != "" {
			doi = d
			e.APA7.DOI = d
			changed = true
		}
	}
	if doi != "" {
		desired := "https://doi.org/" + doi
		if strings.TrimSpace(e.APA7.URL) != desired {
			e.APA7.URL = desired
            // Ensure accessed is set to satisfy validation
            if strings.TrimSpace(e.APA7.Accessed) == "" {
                e.APA7.Accessed = dates.NowISO()
            }
			changed = true
		}
	}
	return changed
}

// FilterByKeywordsAND filters entries to those containing all provided keywords (case-insensitive).
func FilterByKeywordsAND(entries []schema.Entry, keywords []string) []schema.Entry {
	if len(keywords) == 0 {
		return entries
	}
	ks := make([]string, 0, len(keywords))
	for _, k := range keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" {
			ks = append(ks, k)
		}
	}
	var out []schema.Entry
	for _, e := range entries {
		hit := true
		set := map[string]bool{}
		for _, k := range e.Annotation.Keywords {
			set[strings.ToLower(strings.TrimSpace(k))] = true
		}
		for _, k := range ks {
			if !set[k] {
				hit = false
				break
			}
		}
		if hit {
			out = append(out, e)
		}
	}
	return out
}

// (store migration removed)
