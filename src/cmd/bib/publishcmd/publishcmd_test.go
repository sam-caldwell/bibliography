package publishcmd

import (
    "bytes"
    "os"
    "path/filepath"
    "testing"
    "bibliography/src/internal/schema"
    "bibliography/src/internal/store"
)

func TestPublishWritesDocs(t *testing.T) {
    dir := t.TempDir()
    old, _ := os.Getwd()
    t.Cleanup(func(){ _ = os.Chdir(old) })
    _ = os.Chdir(dir)
    e := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "Hello", URL: "https://example", Accessed: "2024-01-01"}, Annotation: schema.Annotation{Summary: "s", Keywords: []string{"k"}}}
    if _, err := store.WriteEntry(e); err != nil { t.Fatal(err) }
    cmd := New()
    var buf bytes.Buffer
    cmd.SetOut(&buf)
    if err := cmd.RunE(cmd, []string{}); err != nil { t.Fatalf("publish run: %v", err) }
    if _, err := os.Stat(filepath.Join("docs", "index.html")); err != nil { t.Fatalf("index.html not written") }
    if _, err := os.Stat(filepath.Join("docs", store.SegmentForType(e.Type), e.ID+".html")); err != nil { t.Fatalf("entry page not written") }
}
