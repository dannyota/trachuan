package taxonomy

import (
	"reflect"
	"testing"
)

func TestInheritanceCandidates_Correspondence(t *testing.T) {
	articles := []InheritanceArticle{
		{ID: 1, DocumentID: 10, Anchor: "dieu-1", Heading: "  ĐIỀU  lệ ", Consolidated: true},
		{ID: 2, DocumentID: 20, Anchor: "dieu-1", Heading: "điều lệ", Seeds: []string{"2.1"}},
		{ID: 3, DocumentID: 30, Anchor: "dieu-1", Heading: "điều lệ", Seeds: []string{"3.1", "2.1"}},
	}
	tests := []struct {
		name      string
		change    func([]InheritanceArticle)
		relations map[int64][]int64
		want      map[int64][]string
	}{
		{"several sources", func([]InheritanceArticle) {}, map[int64][]int64{10: {20, 30}}, map[int64][]string{1: {"2.1", "3.1"}}},
		{"empty heading", func(a []InheritanceArticle) { a[0].Heading = " \t" }, map[int64][]int64{10: {20}}, map[int64][]string{}},
		{"different heading", func(a []InheritanceArticle) { a[1].Heading = "other" }, map[int64][]int64{10: {20}}, map[int64][]string{}},
		{"no relation", func([]InheritanceArticle) {}, nil, map[int64][]string{}},
		{"already seeded", func(a []InheritanceArticle) { a[0].Seeds = []string{"4.1"} }, map[int64][]int64{10: {20}}, map[int64][]string{}},
		{"non numeric anchor", func(a []InheritanceArticle) { a[0].Anchor = "dieu-a"; a[1].Anchor = "dieu-a" }, map[int64][]int64{10: {20}}, map[int64][]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := append([]InheritanceArticle(nil), articles...)
			tt.change(a)
			if got := InheritanceCandidates(a, tt.relations); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
