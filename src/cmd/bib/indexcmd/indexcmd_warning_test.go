package indexcmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestIndexCommand_WarnsWhenNotGitRepo(t *testing.T) {
	cmd := New(func(paths []string, msg string) error { return fmt.Errorf("not a git repository") })
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("expected warning, got error: %v", err)
	}
	if !strings.Contains(stderr.String(), "skipping git commit") {
		t.Fatalf("expected warning on stderr, got: %q", stderr.String())
	}
}
