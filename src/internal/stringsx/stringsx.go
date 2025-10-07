package stringsx

import "strings"

// FirstNonEmpty returns the first non-empty trimmed string.
// FirstNonEmpty returns the first string in vals that is non-empty when trimmed.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
