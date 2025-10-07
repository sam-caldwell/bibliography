package citecmd

import "testing"

func TestJoinOxfordAmp(t *testing.T) {
	if joinOxfordAmp([]string{}) != "" {
		t.Fatalf("empty case")
	}
	if joinOxfordAmp([]string{"A"}) != "A" {
		t.Fatalf("single case")
	}
	if joinOxfordAmp([]string{"A", "B"}) != "A, & B" {
		t.Fatalf("two case")
	}
	if joinOxfordAmp([]string{"A", "B", "C"}) != "A, B, & C" {
		t.Fatalf("many case")
	}
}
