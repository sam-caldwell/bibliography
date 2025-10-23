package main

import (
	"bibliography/src/cmd/bib/citecmd"
	"bibliography/src/cmd/bib/searchcmd"
	"bibliography/src/internal/schema"
	"regexp"
)

// Shims to satisfy existing tests referencing helpers in package main.

func toAPACitation(e schema.Entry) string       { return citecmd.APACitation(e) }
func wildcardToRegex(pat string) *regexp.Regexp { return searchcmd.WildcardToRegex(pat) }
func countContains(text, q string) int          { return searchcmd.CountContains(text, q) }
