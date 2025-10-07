package summarize

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type seqDoer struct {
	bodies []string
	i      int
}

func (s *seqDoer) Do(req *http.Request) (*http.Response, error) {
	if s.i >= len(s.bodies) {
		s.bodies = append(s.bodies, s.bodies[len(s.bodies)-1])
	}
	b := s.bodies[s.i]
	s.i++
	if b == "" {
		b = `{"choices":[{"message":{"content":"[]"}}]}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(b)), Header: make(http.Header)}, nil
}

func TestGenerateMovieAndArticle_Parsers(t *testing.T) {
	// First call (movie object), second call (keywords array)
	mObj := `{"choices":[{"message":{"content":"{\"title\":\"T\",\"date\":\"2020-01-02\",\"publisher\":\"P\",\"authors\":[{\"family\":\"Doe\",\"given\":\"J\"}],\"summary\":\"S\"}"}}]}`
	arr := `{"choices":[{"message":{"content":"[\"a\",\"b\"]"}}]}`
	bodies := []string{mObj, arr, // for movie
		// For article then keywords
		`{"choices":[{"message":{"content":"{\"title\":\"A\",\"journal\":\"J\",\"date\":\"2021-01-01\",\"doi\":\"10.1/x\"}"}}]}`,
		arr,
	}
	old := client
	defer func() { client = old }()
	client = &seqDoer{bodies: bodies}
	t.Setenv("OPENAI_API_KEY", "x")
	if _, err := GenerateMovieFromTitleAndDate(context.Background(), "T", ""); err != nil {
		t.Fatalf("GenerateMovieFromTitleAndDate: %v", err)
	}
	if _, err := GenerateCitationFromURL(context.Background(), "https://x"); err != nil {
		t.Fatalf("GenerateCitationFromURL: %v", err)
	}
}

func TestGenerateSongFromTitleArtistDate(t *testing.T) {
	songObj := `{"choices":[{"message":{"content":"{\"title\":\"S\",\"date\":\"2022-03-04\",\"publisher\":\"L\",\"container_title\":\"Album\",\"authors\":[{\"family\":\"Artist\"}],\"summary\":\"Sum\"}"}}]}`
	arr := `{"choices":[{"message":{"content":"[\"pop\",\"music\"]"}}]}`
	old := client
	defer func() { client = old }()
	client = &seqDoer{bodies: []string{songObj, arr}}
	t.Setenv("OPENAI_API_KEY", "x")
	e, err := GenerateSongFromTitleArtistDate(context.Background(), "S", "A", "")
	if err != nil {
		t.Fatalf("GenerateSong: %v", err)
	}
	if e.Type != "song" || e.APA7.Title == "" || len(e.Annotation.Keywords) == 0 {
		t.Fatalf("bad song entry: %+v", e)
	}
}
