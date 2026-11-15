package build

import (
	"fmt"
	"testing"
)

func TestInheritance_MatchingAndUnion(t *testing.T) {
	nodes := map[int64]taxonomyNode{2: {id: 2, level: 2}, 3: {id: 3, level: 3, parent: 2}, 4: {id: 4, level: 3, parent: 2}}
	articles := []inheritanceArticle{
		{id: 1, doc: 1, anchor: "dieu-1", heading: "  PHẠM\tVI ", seeds: []int64{3}},
		{id: 2, doc: 2, anchor: "dieu-1", heading: "Pha\u0323m vi", seeds: []int64{4}},
		{id: 3, doc: 3, anchor: "dieu-1", heading: "phạm vi", consolidated: true},
		{id: 4, doc: 3, anchor: "dieu-2", heading: "different", consolidated: true},
	}
	got := inheritArticles(articles, map[int64][]int64{3: {1, 2}}, nodes)
	if len(got.assignments[3]) != 2 || got.unmatched != 1 {
		t.Fatalf("result %+v", got)
	}
}

func TestInheritanceGate_Threshold(t *testing.T) {
	for _, tc := range []struct {
		count   int
		correct bool
		want    string
	}{{99, true, "unavailable"}, {100, true, "pass"}, {100, false, "fail"}} {
		articles := []inheritanceArticle{}
		for i := 0; i < tc.count; i++ {
			id := int64(i + 1)
			seed := int64(3)
			if !tc.correct {
				seed = 4
			}
			articles = append(articles, inheritanceArticle{id: id, doc: 2, anchor: fmt.Sprintf("dieu-%d", i), heading: "same", consolidated: true, seeds: []int64{seed}}, inheritanceArticle{id: id + 1000, doc: 1, anchor: fmt.Sprintf("dieu-%d", i), heading: "same", seeds: []int64{3}})
		}
		nodes := map[int64]taxonomyNode{2: {id: 2, level: 2}, 3: {id: 3, level: 3, parent: 2}, 4: {id: 4, level: 3, parent: 2}}
		got := inheritArticles(articles, map[int64][]int64{2: {1}}, nodes).gate
		if got.Status != tc.want {
			t.Fatalf("count %d correct %v status %s, want %s", tc.count, tc.correct, got.Status, tc.want)
		}
	}
}

func TestInheritance_ArticleCases(t *testing.T) {
	nodes := map[int64]taxonomyNode{2: {id: 2, level: 2}, 3: {id: 3, level: 3, parent: 2}}
	for _, tc := range []struct {
		name, sourceHeading, targetHeading, anchor string
		seed                                       bool
		matched                                    bool
	}{
		{"equal", " heading ", "HEADING", "dieu-1", false, true},
		{"different", "one", "two", "dieu-1", false, false},
		{"empty source", "", "heading", "dieu-1", false, false},
		{"empty target", "heading", " \t", "dieu-1", false, false},
		{"missing", "heading", "heading", "dieu-2", false, false},
		{"non-numeric", "heading", "heading", "dieu-1a", false, false},
		{"direct seed", "heading", "heading", "dieu-1", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := inheritanceArticle{id: 2, doc: 2, anchor: tc.anchor, heading: tc.targetHeading, consolidated: true}
			if tc.seed {
				target.seeds = []int64{3}
			}
			sourceAnchor := "dieu-1"
			if tc.name == "non-numeric" {
				sourceAnchor = tc.anchor
			}
			articles := []inheritanceArticle{{id: 1, doc: 1, anchor: sourceAnchor, heading: tc.sourceHeading, seeds: []int64{3}}, target}
			got := inheritArticles(articles, map[int64][]int64{2: {1}}, nodes)
			if (len(got.assignments[2]) > 0) != tc.matched {
				t.Fatalf("assignments: %v", got.assignments)
			}
		})
	}
}

func TestInheritanceGate_MicroPrecisionAtFloors(t *testing.T) {
	nodes := map[int64]taxonomyNode{
		2: {id: 2, level: 2}, 3: {id: 3, level: 3, parent: 2}, 4: {id: 4, level: 4, parent: 3},
		5: {id: 5, level: 4, parent: 3}, 6: {id: 6, level: 3, parent: 2}, 7: {id: 7, level: 2},
	}
	var articles []inheritanceArticle
	for i := 0; i < 100; i++ {
		truth := int64(4)
		if i >= 95 {
			truth = 7
		} else if i >= 90 {
			truth = 6
		} else if i >= 85 {
			truth = 5
		}
		anchor := fmt.Sprintf("dieu-%d", i+1)
		articles = append(articles, inheritanceArticle{id: int64(i + 1), doc: 1, anchor: anchor, heading: "same", seeds: []int64{4}}, inheritanceArticle{id: int64(i + 101), doc: 2, anchor: anchor, heading: "same", consolidated: true, seeds: []int64{truth}})
	}
	gate := inheritArticles(articles, map[int64][]int64{2: {1}}, nodes).gate
	if gate.Status != "pass" || gate.Articles != 100 {
		t.Fatalf("gate: %+v", gate)
	}
	for i, want := range []int{95, 90, 85} {
		if gate.Rows[i].Numerator != want || gate.Rows[i].Denominator != 100 {
			t.Fatalf("row %d: %+v", i, gate.Rows[i])
		}
	}
}

func TestInheritanceGate_MicroPrecisionDeduplicatesSources(t *testing.T) {
	nodes := map[int64]taxonomyNode{
		2: {id: 2, level: 2}, 3: {id: 3, level: 3, parent: 2},
		4: {id: 4, level: 4, parent: 3}, 5: {id: 5, level: 4, parent: 3},
		6: {id: 6, level: 2}, 7: {id: 7, level: 3, parent: 6}, 8: {id: 8, level: 4, parent: 7},
	}
	var articles []inheritanceArticle
	for i := 1; i <= 100; i++ {
		anchor := fmt.Sprintf("dieu-%d", i)
		source, target := []int64{4}, []int64{4}
		if i == 1 {
			source = []int64{4, 4, 5}
			target = []int64{4, 8}
		}
		if i == 2 {
			source = []int64{8}
		}
		articles = append(articles,
			inheritanceArticle{id: int64(i), doc: 1, anchor: anchor, heading: "same", seeds: source},
			inheritanceArticle{id: int64(i + 200), doc: 3, anchor: anchor, heading: "same", consolidated: true, seeds: target})
		if i == 1 {
			articles = append(articles, inheritanceArticle{id: 101, doc: 2, anchor: anchor, heading: "same", seeds: []int64{4, 8, 8}})
		}
	}
	gate := inheritArticles(articles, map[int64][]int64{3: {1, 2}}, nodes).gate
	if gate.Articles != 100 {
		t.Fatalf("articles=%d", gate.Articles)
	}
	for i, want := range []struct{ numerator, denominator int }{{100, 101}, {100, 101}, {100, 102}} {
		row := gate.Rows[i]
		if row.Numerator != want.numerator || row.Denominator != want.denominator {
			t.Fatalf("%s=%d/%d, want %d/%d", row.Name, row.Numerator, row.Denominator, want.numerator, want.denominator)
		}
	}
}
