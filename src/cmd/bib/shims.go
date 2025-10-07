package main

import (
    "regexp"
    "gopkg.in/yaml.v3"
    "bibliography/src/internal/schema"
    "bibliography/src/cmd/bib/citecmd"
    "bibliography/src/cmd/bib/searchcmd"
    "bibliography/src/cmd/bib/editcmd"
)

// Shims to satisfy existing tests referencing helpers in package main.

func toAPACitation(e schema.Entry) string { return citecmd.APACitation(e) }
func wildcardToRegex(pat string) *regexp.Regexp { return searchcmd.WildcardToRegex(pat) }
func countContains(text, q string) int { return searchcmd.CountContains(text, q) }
func setYAMLPathValue(root *yaml.Node, dotPath string, raw string) error { return editcmd.SetYAMLPathValue(root, dotPath, raw) }
func splitDotPath(p string) ([]string, error) { return editcmd.SplitDotPath(p) }
