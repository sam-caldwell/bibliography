package editcmd

import "testing"

func TestParseEditArgs_Variants(t *testing.T) {
	id, assigns, err := parseEditArgs([]string{"--id", "abc", "--apa7.title=New", "annotation.keywords=[k]"})
	if err != nil || id != "abc" || assigns["apa7.title"] == "" || assigns["annotation.keywords"] == "" {
		t.Fatalf("parse variants failed: id=%q assigns=%+v err=%v", id, assigns, err)
	}
}

func TestDisallowIDEdits(t *testing.T) {
	if err := disallowIDEdits(map[string]string{"id": "x"}); err == nil {
		t.Fatalf("expected id edit error")
	}
}
