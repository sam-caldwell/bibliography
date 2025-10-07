package citecmd

import (
	"strings"
	"testing"

	"bibliography/src/internal/schema"
)

func TestAPACitation_TypeDetails(t *testing.T) {
	y := 2021
	cases := []struct {
		name string
		e    schema.Entry
		want []string
	}{
		{"article", schema.Entry{Type: "article", APA7: schema.APA7{Title: "T", Year: &y, Journal: "J", Volume: "10", Issue: "2", Pages: "1-3"}}, []string{"J", "10(2)", "1-3"}},
		{"book", schema.Entry{Type: "book", APA7: schema.APA7{Title: "B", Year: &y, Publisher: "Pub"}}, []string{"Pub"}},
		{"website", schema.Entry{Type: "website", APA7: schema.APA7{Title: "W", Year: &y, ContainerTitle: "Site"}}, []string{"Site"}},
		{"movie", schema.Entry{Type: "movie", APA7: schema.APA7{Title: "M", Year: &y, Publisher: "Studio"}}, []string{"[Film]", "Studio"}},
		{"video", schema.Entry{Type: "video", APA7: schema.APA7{Title: "V", Year: &y}}, []string{"[Video]", "YouTube"}},
		{"song", schema.Entry{Type: "song", APA7: schema.APA7{Title: "S", Year: &y, ContainerTitle: "Album", Publisher: "Label"}}, []string{"[Song]", "Album", "Label"}},
		{"patent", schema.Entry{Type: "patent", APA7: schema.APA7{Title: "P", Year: &y, ContainerTitle: "USPTO", Publisher: "Assignee"}}, []string{"Patent office: USPTO", "Assignee"}},
		{"rfc", schema.Entry{Type: "rfc", APA7: schema.APA7{Title: "R", Year: &y}}, []string{"RFC"}},
	}
	for _, tc := range cases {
		got := APACitation(tc.e)
		for _, sub := range tc.want {
			if !strings.Contains(got, sub) {
				t.Fatalf("%s: missing %q in %q", tc.name, sub, got)
			}
		}
	}
}
