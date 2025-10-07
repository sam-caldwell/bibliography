package searchcmd

import (
	"bytes"
	"testing"
)

func TestRenderTable(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"id", "type", "title"}
	rows := [][]string{{"1", "book", "B"}, {"2", "site", "S"}}
	renderTable(&buf, headers, rows)
	out := buf.String()
	if out == "" || !bytes.Contains(buf.Bytes(), []byte("id")) {
		t.Fatalf("renderTable output empty: %q", out)
	}
}
