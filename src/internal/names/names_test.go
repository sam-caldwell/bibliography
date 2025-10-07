package names

import "testing"

func TestInitials(t *testing.T) {
    if got := Initials("Jane Q"); got != "J. Q." {
        t.Fatalf("Initials: want 'J. Q.', got %q", got)
    }
    if got := Initials(""); got != "" {
        t.Fatalf("Initials empty: want '', got %q", got)
    }
}

func TestSplit(t *testing.T) {
    fam, giv := Split("Doe, Jane Q")
    if fam != "Doe" || giv != "J. Q." {
        t.Fatalf("Split comma: got (%q,%q)", fam, giv)
    }
    fam, giv = Split("Jane Quimby Doe")
    if fam != "Doe" || giv != "J. Q." {
        t.Fatalf("Split space: got (%q,%q)", fam, giv)
    }
}

