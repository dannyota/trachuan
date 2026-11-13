package textparse

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse_OverlappingChildTextStaysWhole(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"clause", "Dẫn \"\n1. X\"\n1. X\"\n1. X\"\n2. Y"},
		{"point in clause", "1. Khoản một\nDẫn \"\na) X\"\na) X\"\na) X\"\nb) Y\n2. Khoản hai"},
		{"point in article", "Dẫn \"\na) X\"\na) X\"\na) X\"\nb) Y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := paragraphs(strings.Repeat("Lời mở đầu ", 40) + "\nĐiều 1. Nội dung\n" + tc.body)
			got := Parse(ps)
			if len(got.Scopes) != 1 || !got.Scopes[0].Accepted || len(got.Gaps) != 0 {
				t.Fatalf("scopes=%+v gaps=%v", got.Scopes, got.Gaps)
			}
			if !reflect.DeepEqual(got.Provisions, ParseArticles(ps).Provisions) {
				t.Error("overlapping child text must leave the article whole")
			}
			counts := got.Counts.Structure
			if counts == nil || counts.ArticlesSplit != 0 || counts.Clauses != 0 || counts.Points != 0 || !reflect.DeepEqual(counts.Reasons, map[string]int{"containment": 1}) {
				t.Fatalf("counts=%+v", counts)
			}
		})
	}
}
