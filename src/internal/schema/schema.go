package schema

import (
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry represents a single citation entry stored on disk as YAML.
type Entry struct {
	ID         string     `yaml:"id" json:"id"`
	Type       string     `yaml:"type" json:"type"`
	APA7       APA7       `yaml:"apa7" json:"apa7"`
	Annotation Annotation `yaml:"annotation" json:"annotation"`
}

// APA7 holds bibliographic fields (subset as per spec).
type APA7 struct {
	Authors           Authors `yaml:"authors" json:"authors"`
	Year              *int    `yaml:"year,omitempty" json:"year,omitempty"`
	Date              string  `yaml:"date,omitempty" json:"date,omitempty"`
	Title             string  `yaml:"title" json:"title"`
	ContainerTitle    string  `yaml:"container_title,omitempty" json:"container_title,omitempty"`
	Edition           string  `yaml:"edition,omitempty" json:"edition,omitempty"`
	Publisher         string  `yaml:"publisher,omitempty" json:"publisher,omitempty"`
	PublisherLocation string  `yaml:"publisher_location,omitempty" json:"publisher_location,omitempty"`
	Journal           string  `yaml:"journal,omitempty" json:"journal,omitempty"`
	Volume            string  `yaml:"volume,omitempty" json:"volume,omitempty"`
	Issue             string  `yaml:"issue,omitempty" json:"issue,omitempty"`
	Pages             string  `yaml:"pages,omitempty" json:"pages,omitempty"`
	DOI               string  `yaml:"doi,omitempty" json:"doi,omitempty"`
	ISBN              string  `yaml:"isbn,omitempty" json:"isbn,omitempty"`
	URL               string  `yaml:"url,omitempty" json:"url,omitempty"`
	BibTeXURL         string  `yaml:"bibtex_url,omitempty" json:"bibtex_url,omitempty"`
	Accessed          string  `yaml:"accessed,omitempty" json:"accessed,omitempty"`
}

type Author struct {
	Family string `yaml:"family" json:"family"`
	Given  string `yaml:"given,omitempty" json:"given,omitempty"`
}

type Annotation struct {
	Summary  string   `yaml:"summary" json:"summary"`
	Keywords []string `yaml:"keywords" json:"keywords"`
}

// Authors is a slice of Author that can unmarshal from multiple YAML shapes:
// - a single string (treated as a corporate or full-name author; stored in Family)
// - a sequence of strings
// - a mapping (single Author object)
// - a sequence of Author mappings
type Authors []Author

func (a *Authors) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		*a = nil
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		// Single string author (e.g., corporate author)
		s := strings.TrimSpace(value.Value)
		if s == "" || s == "null" {
			*a = nil
			return nil
		}
		*a = Authors{{Family: s}}
		return nil
	case yaml.SequenceNode:
		// Could be sequence of strings or sequence of mappings
		var out Authors
		for _, n := range value.Content {
			if n.Kind == yaml.ScalarNode {
				s := strings.TrimSpace(n.Value)
				if s == "" {
					continue
				}
				out = append(out, Author{Family: s})
				continue
			}
			if n.Kind == yaml.MappingNode {
				au := parseAuthorMapping(n)
				if strings.TrimSpace(au.Family) == "" && strings.TrimSpace(au.Given) == "" {
					continue
				}
				out = append(out, au)
				continue
			}
		}
		*a = out
		return nil
	case yaml.MappingNode:
		// Single author object
		au := parseAuthorMapping(value)
		if strings.TrimSpace(au.Family) == "" && strings.TrimSpace(au.Given) == "" {
			*a = nil
			return nil
		}
		*a = Authors{au}
		return nil
	default:
		// Unknown shape; leave nil rather than erroring
		*a = nil
		return nil
	}
}

// parseAuthorMapping accepts either standard APA7 author mapping with
// family/given keys, or a corporate/organization mapping with keys like
// organization, org, corporate, or name. It normalizes those to Author{Family: name}.
func parseAuthorMapping(n *yaml.Node) Author {
	var au Author
	// Try direct decode first (family/given)
	_ = n.Decode(&au)
	fam := strings.TrimSpace(au.Family)
	giv := strings.TrimSpace(au.Given)
	if fam != "" || giv != "" {
		return Author{Family: fam, Given: giv}
	}
	// Fallback: scan mapping for alternate keys
	if n.Kind != yaml.MappingNode {
		return Author{}
	}
	// Build a simple key->value map
	m := map[string]string{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := strings.ToLower(strings.TrimSpace(n.Content[i].Value))
		v := strings.TrimSpace(n.Content[i+1].Value)
		m[k] = v
	}
	if s := strings.TrimSpace(m["family"]); s != "" {
		fam = s
	}
	if s := strings.TrimSpace(m["given"]); s != "" {
		giv = s
	}
	if fam == "" && giv == "" {
		// Accept org/name variants for corporate authors
		if s := strings.TrimSpace(m["organization"]); s != "" {
			fam = s
		} else if s := strings.TrimSpace(m["org"]); s != "" {
			fam = s
		} else if s := strings.TrimSpace(m["corporate"]); s != "" {
			fam = s
		} else if s := strings.TrimSpace(m["name"]); s != "" {
			fam = s
		}
	}
	return Author{Family: fam, Given: giv}
}

// Validate applies basic validation rules from specification.
func (e *Entry) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("id is required")
	}
	// Enforce UUIDv4 id with fixed 36-char canonical form
	if !isUUIDv4(e.ID) {
		return fmt.Errorf("id must be uuidv4 (36-char canonical), got %q", e.ID)
	}
	switch e.Type {
	case "website", "book", "movie", "video", "song", "article", "patent", "report", "dataset", "software", "rfc":
	default:
		return fmt.Errorf("invalid type: %s", e.Type)
	}
	if strings.TrimSpace(e.APA7.Title) == "" {
		return errors.New("apa7.title is required")
	}
	if strings.TrimSpace(e.Annotation.Summary) == "" {
		return errors.New("annotation.summary is required")
	}
	if len(e.Annotation.Keywords) == 0 {
		return errors.New("annotation.keywords must have at least one keyword")
	}
	if strings.TrimSpace(e.APA7.URL) != "" && strings.TrimSpace(e.APA7.Accessed) == "" {
		return errors.New("apa7.accessed is required when apa7.url is present")
	}
	return nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
var dashCollapse = regexp.MustCompile(`-+`)

// Slugify generates an id-friendly slug from title and optional year.
func Slugify(title string, year *int) string {
	t := strings.ToLower(strings.TrimSpace(title))
	t = nonAlnum.ReplaceAllString(t, "-")
	t = dashCollapse.ReplaceAllString(t, "-")
	t = strings.Trim(t, "-")
	if year != nil {
		return fmt.Sprintf("%s-%d", t, *year)
	}
	return t
}

// NewID returns a new UUIDv4 string in canonical 8-4-4-4-12 form.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// Set version (4) and variant (10xx)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hex := func(x byte) byte { const hexd = "0123456789abcdef"; return hexd[x] }
	dst := make([]byte, 36)
	pos := 0
	writeByte := func(x byte) { dst[pos] = x; pos++ }
	writeHex := func(x byte) { writeByte(hex(x >> 4)); writeByte(hex(x & 0x0f)) }
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			writeByte('-')
		}
		writeHex(v)
	}
	return string(dst)
}

var reUUIDv4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func isUUIDv4(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return reUUIDv4.MatchString(s)
}
