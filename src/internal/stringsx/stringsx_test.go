package stringsx

import "testing"

func TestFirstNonEmpty(t *testing.T) {
    if got := FirstNonEmpty("", " ", "x", "y"); got != "x" {
        t.Fatalf("FirstNonEmpty: want 'x', got %q", got)
    }
    if got := FirstNonEmpty("", ""); got != "" {
        t.Fatalf("FirstNonEmpty empty: want '', got %q", got)
    }
}

