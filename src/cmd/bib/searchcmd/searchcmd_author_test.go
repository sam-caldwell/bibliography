package searchcmd

import (
	"bytes"
	"testing"

	"bibliography/src/internal/schema"
)

func TestAuthorEqualsWildcard(t *testing.T) {
	y := 2020
	e := schema.Entry{Type: "article", APA7: schema.APA7{Title: "T", Year: &y, Authors: schema.Authors{{Family: "Doe", Given: "Jane"}}}}
	p, ok, err := compileAuthorEqualsTerm("author==doe*")
	if err != nil || !ok {
		t.Fatalf("compile author wildcard: ok=%v err=%v", ok, err)
	}
	hit, score := p(e)
	if !hit || score == 0 {
		t.Fatalf("expected hit with score, got hit=%v score=%d", hit, score)
	}

	// Render with this single result to exercise table writer
	var buf bytes.Buffer
	renderTable(&buf, []string{"id", "type", "title", "author"}, [][]string{{"1", "article", "T", "Doe"}})
	if buf.Len() == 0 {
		t.Fatalf("expected table output")
	}
}
