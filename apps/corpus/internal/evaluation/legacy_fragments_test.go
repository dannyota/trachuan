package evaluation

import (
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestMatchLegacy_CompleteFragments(t *testing.T) {
	fragments := []retrieval.Fragment{
		{Complete: true, Citation: "Điều 11, Khoản 1"},
		{Complete: true, Citation: "Điều 11, Khoản 2"},
		{Complete: true, Citation: "Điều 11, Khoản 2, Điểm a"},
		{Complete: false, Citation: "Điều 11, Khoản 3"},
	}
	hit := retrieval.Hit{Number: "12/AB", Citation: "Điều 11", Fragments: fragments}
	for _, tc := range []struct {
		name string
		e    Expectation
		want int
	}{
		{"article", Expectation{DocNumber: "12/AB", Article: "11"}, 2},
		{"clause", Expectation{DocNumber: "12/AB", Article: "11", Clause: "1"}, 2},
		{"point", Expectation{DocNumber: "12/AB", Article: "11", Clause: "2", Point: "a"}, 2},
		{"alternate", Expectation{DocNumber: "99", AltDocNumbers: []string{"12/AB"}, Article: "11", Clause: "2"}, 2},
		{"wrong clause", Expectation{DocNumber: "12/AB", Article: "11", Clause: "4"}, 0},
		{"truncated", Expectation{DocNumber: "12/AB", Article: "11", Clause: "3"}, 0},
		{"wrong number", Expectation{DocNumber: "99", Article: "11", Clause: "1"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rank, relation := MatchLegacy(tc.e, []retrieval.Hit{{Number: "other"}, hit})
			if rank != tc.want || relation {
				t.Fatalf("rank %d relation %t, want %d false", rank, relation, tc.want)
			}
		})
	}
}

func TestMatchLegacy_ParentHitCreditsCompleteChild(t *testing.T) {
	f := retrieval.Fragment{Identity: retrieval.Identity{Country: "vn", DocumentKey: "d", Anchor: "child"}, Complete: true, Citation: "Điều 11, Khoản 1"}
	h := retrieval.Hit{Identity: retrieval.Identity{Country: "vn", DocumentKey: "d", Anchor: "parent"}, Number: "12", Citation: "Điều 11", Fragments: []retrieval.Fragment{f}}
	anchor := "child"
	product := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "d", Anchor: &anchor, Kind: "provision"}}}
	if rank, relation := MatchLegacy(Expectation{DocNumber: "12", Article: "11", Clause: "1"}, []retrieval.Hit{h}); rank != 1 || relation || MatchProduct(product, []retrieval.Hit{h}) != 1 {
		t.Fatalf("parent child credit: rank %d relation %t", rank, relation)
	}
}
