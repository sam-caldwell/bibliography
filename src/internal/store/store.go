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
	"time"

	"gopkg.in/yaml.v3"

	"bibliography/src/internal/schema"
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

// dirForType maps entry types to subdirectories under data/citations.
// Unknown types fall back to "citation". Pluralization/aliases per request.
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

// SegmentForType exposes the directory segment used for a given type.
// E.g., "book" -> "books", "website" -> "site".
func SegmentForType(typ string) string { return dirForType(typ) }

// WriteEntry writes the entry YAML to data/citations/<id>.yaml after validation.
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

// ReadAll loads all entries from data/citations.
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

// BuildKeywordIndex builds a map of keyword -> list of entry IDs and writes to JSON.
func BuildKeywordIndex(entries []schema.Entry) (string, error) {
	if err := os.MkdirAll(MetadataDir, 0o755); err != nil {
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
			seg := dirForType(e.Type)
			path := filepath.ToSlash(filepath.Join(CitationsDir, seg, e.ID+".yaml"))
			index[t] = append(index[t], path)
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
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(KeywordsJSON, b, 0o644); err != nil {
		return "", err
	}
	return KeywordsJSON, nil
}

// BuildAuthorIndex builds a map of author name -> list of full repo-relative
// file paths for works they authored, and writes it to JSON.
// Author key format: "Family, Given" if both present, else the non-empty name.
func BuildAuthorIndex(entries []schema.Entry) (string, error) {
	if err := os.MkdirAll(MetadataDir, 0o755); err != nil {
		return "", err
	}
	index := map[string][]string{}
	for _, e := range entries {
		seg := dirForType(e.Type)
		path := filepath.ToSlash(filepath.Join(CitationsDir, seg, e.ID+".yaml"))
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
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(AuthorsJSON, b, 0o644); err != nil {
		return "", err
	}
	return AuthorsJSON, nil
}

// BuildTitleIndex builds a map of full repo-relative file path -> list of
// tokenized title words for every cited work and writes it to JSON.
func BuildTitleIndex(entries []schema.Entry) (string, error) {
	if err := os.MkdirAll(MetadataDir, 0o755); err != nil {
		return "", err
	}
	index := map[string][]string{}
	for _, e := range entries {
		seg := dirForType(e.Type)
		path := filepath.ToSlash(filepath.Join(CitationsDir, seg, e.ID+".yaml"))
		index[path] = tokenizeWords(e.APA7.Title)
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(TitlesJSON, b, 0o644); err != nil {
		return "", err
	}
	return TitlesJSON, nil
}

// BuildISBNIndex builds a map of full repo-relative file path -> ISBN for
// every cited book and writes it to JSON. Entries without ISBN are skipped.
func BuildISBNIndex(entries []schema.Entry) (string, error) {
	if err := os.MkdirAll(MetadataDir, 0o755); err != nil {
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
		seg := dirForType(e.Type)
		path := filepath.ToSlash(filepath.Join(CitationsDir, seg, e.ID+".yaml"))
		index[path] = isbn
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(ISBNJSON, b, 0o644); err != nil {
		return "", err
	}
	return ISBNJSON, nil
}

// BuildDOIIndex builds a map of full repo-relative file path -> DOI for
// every cited book and writes it to JSON. Entries without DOI are skipped.
func BuildDOIIndex(entries []schema.Entry) (string, error) {
	if err := os.MkdirAll(MetadataDir, 0o755); err != nil {
		return "", err
	}
	index := map[string]string{}
	for _, e := range entries {
		doi := strings.TrimSpace(e.APA7.DOI)
		if doi == "" {
			continue
		}
		seg := dirForType(e.Type)
		path := filepath.ToSlash(filepath.Join(CitationsDir, seg, e.ID+".yaml"))
		index[path] = doi
	}
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(DOIJSON, b, 0o644); err != nil {
		return "", err
	}
	return DOIJSON, nil
}

var nonWord = regexp.MustCompile(`[^a-zA-Z0-9]+`)
var doiRegex = regexp.MustCompile(`(?i)10\.\d{4,9}/[-._;()/:A-Z0-9]+`)

// tokenizeWords splits a phrase into lowercased word tokens, filtering empties and 1-char tokens.
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

// ExtractDOI tries to extract a DOI from an arbitrary string (URL or text).
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

// NormalizeArticleDOI ensures that if a DOI is available, the entry has
// apa7.doi set and apa7.url points to https://doi.org/<doi>. It will extract
// a DOI from the existing URL if missing. Returns true if modified.
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
				e.APA7.Accessed = time.Now().UTC().Format("2006-01-02")
			}
			changed = true
		}
	}
	return changed
}

// FilterByKeywordsAND returns entries whose annotation.keywords contains all keywords (case-insensitive).
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
