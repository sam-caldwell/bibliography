package names

import (
    "strings"
)

// Initials converts a given name string into spaced initials: "Jane Q" -> "J. Q.".
func Initials(given string) string {
    given = strings.TrimSpace(given)
    if given == "" {
        return ""
    }
    var out []string
    for _, w := range strings.Fields(given) {
        r := []rune(w)
        if len(r) == 0 {
            continue
        }
        out = append(out, strings.ToUpper(string(r[0]))+".")
    }
    return strings.Join(out, " ")
}

// Split splits a full name into (family, givenInitials). It accepts either
// "Family, Given Names" or "Given Names Family" and returns initials for given.
func Split(name string) (family, givenInitials string) {
    name = strings.TrimSpace(name)
    if name == "" {
        return "", ""
    }
    if i := strings.Index(name, ","); i >= 0 {
        family = strings.TrimSpace(name[:i])
        given := strings.TrimSpace(name[i+1:])
        return family, Initials(given)
    }
    parts := strings.Fields(name)
    if len(parts) == 1 {
        return parts[0], ""
    }
    family = parts[len(parts)-1]
    given := strings.Join(parts[:len(parts)-1], " ")
    return family, Initials(given)
}

