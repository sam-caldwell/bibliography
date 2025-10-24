package verifycmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"bibliography/src/internal/schema"
)

func TestTableHelpersAndYAMLPreview(t *testing.T) {
	headers := []string{"id", "type", "title", "author"}
	rows := [][]string{{"1", "article", "A", "Doe, J"}, {"2", "book", "B", ""}}
	w := computeColWidths(headers, rows)
	if len(w) != 4 || w[0] < len("id") || w[2] < len("title") {
		t.Fatalf("bad widths: %+v", w)
	}
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	writeColumns(cmd, headers, w)
	writeSeparator(cmd, w)
	for _, r := range rows {
		writeColumns(cmd, r, w)
	}
	out := buf.String()
	if !strings.Contains(out, "id  type") || !strings.Contains(out, "----") {
		t.Fatalf("unexpected table output: %q", out)
	}

	y := 2020
	e := schema.Entry{ID: "x", Type: "article", APA7: schema.APA7{Title: "T", Year: &y, DOI: "10.1/x", URL: "https://doi.org/10.1/x", Accessed: "2025-01-01", Authors: schema.Authors{{Family: "Doe", Given: "J"}}}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k1", "k2"}}}
	preview := entryToYAML(e)
	if !strings.Contains(preview, "id: x") || !strings.Contains(preview, "apa7:") || !strings.Contains(preview, "keywords:") {
		t.Fatalf("yaml preview missing content: %q", preview)
	}
	if a := firstAuthor(e); a != "Doe, J" {
		t.Fatalf("firstAuthor: %q", a)
	}
}
