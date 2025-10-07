package addcmd

import (
    "os"
    "path/filepath"
    "testing"
)

func TestAddWithKeywords_WritesWebsite(t *testing.T) {
    dir := t.TempDir()
    old, _ := os.Getwd()
    t.Cleanup(func(){ _ = os.Chdir(old) })
    _ = os.Chdir(dir)
    called := 0
    commit := func(paths []string, msg string) error { called++; return nil }
    if err := AddWithKeywords(nil, commit, "website", map[string]string{"url": "https://example.com"}, []string{"web"}); err != nil {
        t.Fatalf("AddWithKeywords: %v", err)
    }
    files, _ := os.ReadDir(filepath.Join("data", "citations", "site"))
    if len(files) != 1 { t.Fatalf("expected one site yaml written") }
    if called == 0 { t.Fatalf("expected commit to be called") }
}
