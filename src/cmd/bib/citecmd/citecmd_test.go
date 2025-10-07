package citecmd

import (
    "strings"
    "testing"
    "bibliography/src/internal/schema"
)

func TestAPACitation_VariousTypes(t *testing.T) {
    // Book
    be := schema.Entry{Type: "book", APA7: schema.APA7{Title: "B", Publisher: "P"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    if c := APACitation(be); !strings.Contains(c, "P") { t.Fatalf("book cite missing publisher: %q", c) }
    // Website
    we := schema.Entry{Type: "website", APA7: schema.APA7{Title: "W", ContainerTitle: "Site", URL: "https://x", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    if c := APACitation(we); !strings.Contains(c, "Site") { t.Fatalf("website cite missing container: %q", c) }
    // Movie
    me := schema.Entry{Type: "movie", APA7: schema.APA7{Title: "M"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    if c := APACitation(me); !strings.Contains(c, "[Film]") { t.Fatalf("movie cite missing [Film]: %q", c) }
    // Video (YouTube default)
    ve := schema.Entry{Type: "video", APA7: schema.APA7{Title: "V"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    if c := APACitation(ve); !strings.Contains(c, "[Video]") { t.Fatalf("video cite missing [Video]: %q", c) }
    // Song
    se := schema.Entry{Type: "song", APA7: schema.APA7{Title: "S", ContainerTitle: "Album", Publisher: "Label"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    sc := APACitation(se)
    if !(strings.Contains(sc, "Album") || strings.Contains(sc, "Label")) { t.Fatalf("song cite missing details: %q", sc) }
    // Patent
    pe := schema.Entry{Type: "patent", APA7: schema.APA7{Title: "P", Publisher: "USPTO"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    if c := APACitation(pe); !strings.Contains(c, "USPTO") { t.Fatalf("patent cite missing publisher: %q", c) }
    // RFC
    re := schema.Entry{Type: "rfc", APA7: schema.APA7{Title: "R", ContainerTitle: "RFC"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    if c := APACitation(re); !strings.Contains(c, "RFC") { t.Fatalf("rfc cite missing RFC: %q", c) }
}

