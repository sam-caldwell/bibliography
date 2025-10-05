package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"bibliography/src/internal/schema"
)

const (
	CitationsDir = "data/citations"
	MetadataDir  = "data/metadata"
	KeywordsJSON = "data/metadata/keywords.json"
)

// WriteEntry writes the entry YAML to data/citations/<id>.yaml after validation.
func WriteEntry(e schema.Entry) (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(CitationsDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(CitationsDir, fmt.Sprintf("%s.yaml", e.ID))
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
		for _, k := range e.Annotation.Keywords {
			k2 := strings.ToLower(strings.TrimSpace(k))
			if k2 == "" || seen[k2] {
				continue
			}
			seen[k2] = true
			index[k2] = append(index[k2], e.ID)
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
