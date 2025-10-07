package searchcmd

import (
	"bibliography/src/internal/schema"
	"testing"
)

func TestWildcardAndCount(t *testing.T) {
	rx := WildcardToRegex("doe*")
	if !rx.MatchString("doe, j") || rx.MatchString("roe") {
		t.Fatalf("WildcardToRegex incorrect")
	}
	if n := CountContains("hello hello", "hello"); n != 2 {
		t.Fatalf("CountContains want 2 got %d", n)
	}
}

func TestParseExprPredicates(t *testing.T) {
	preds, err := parseExpr("keyword==go && title~=intro && author==doe*")
	if err != nil {
		t.Fatalf("parseExpr: %v", err)
	}
	e := schema.Entry{Type: "article", APA7: schema.APA7{Title: "An Intro to Go", Authors: schema.Authors{{Family: "Doe", Given: "Jane"}}}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"Go", "Programming"}}}
	hit := true
	score := 0
	for _, p := range preds {
		h, s := p(e)
		if !h {
			hit = false
			break
		}
		score += s
	}
	if !hit {
		t.Fatalf("expected predicates to match entry; score=%d", score)
	}
	// Negative path
	e.APA7.Title = "An Intro to Rust"
	e.Annotation.Keywords = []string{"rust"}
	hit2 := true
	for _, p := range preds {
		h, _ := p(e)
		if !h {
			hit2 = false
			break
		}
	}
	if hit2 {
		t.Fatalf("expected predicates to fail with changed title")
	}
}
