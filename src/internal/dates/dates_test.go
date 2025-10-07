package dates

import (
    "regexp"
    "testing"
    "time"
)

func TestYearFromDate(t *testing.T) {
    if got := YearFromDate("2020-05-01"); got != 2020 {
        t.Fatalf("YearFromDate: want 2020, got %d", got)
    }
    if got := YearFromDate("1999"); got != 1999 {
        t.Fatalf("YearFromDate short: want 1999, got %d", got)
    }
    if got := YearFromDate(""); got != 0 {
        t.Fatalf("YearFromDate empty: want 0, got %d", got)
    }
}

func TestExtractYear(t *testing.T) {
    y := ExtractYear("Published in 1987 by X")
    if y != 1987 {
        t.Fatalf("ExtractYear: want 1987, got %d", y)
    }
    // Should not return years far in the future
    y2 := ExtractYear("year 9999")
    if y2 != 0 {
        t.Fatalf("ExtractYear invalid: want 0, got %d", y2)
    }
}

func TestNowISO(t *testing.T) {
    today := time.Now().UTC().Format("2006-01-02")
    got := NowISO()
    re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
    if !re.MatchString(got) {
        t.Fatalf("NowISO not in YYYY-MM-DD: %q", got)
    }
    if got != today {
        t.Fatalf("NowISO not today: got %q want %q", got, today)
    }
}

