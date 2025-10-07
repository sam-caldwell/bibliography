package dates

import (
	"fmt"
	"strings"
	"time"
)

// YearFromDate parses the first 4 characters of a YYYY or YYYY-MM-DD string.
func YearFromDate(date string) int {
	date = strings.TrimSpace(date)
	if len(date) >= 4 {
		var y int
		if _, err := fmt.Sscanf(date[:4], "%d", &y); err == nil {
			return y
		}
	}
	return 0
}

// ExtractYear scans a string and returns a plausible 4-digit year if found.
func ExtractYear(s string) int {
	s = strings.TrimSpace(s)
	for i := 0; i+4 <= len(s); i++ {
		var y int
		if _, err := fmt.Sscanf(s[i:i+4], "%d", &y); err == nil {
			if y >= 1000 && y <= time.Now().Year()+1 {
				return y
			}
		}
	}
	return 0
}

// NowISO returns the current UTC date as YYYY-MM-DD.
func NowISO() string { return time.Now().UTC().Format("2006-01-02") }
