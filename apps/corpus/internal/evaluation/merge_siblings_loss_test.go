package evaluation

import (
	"context"
	"fmt"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func siblingLossFixture(t *testing.T) (retrieval.Arms, retrieval.Recipe, *retrieval.SiblingMerge) {
	t.Helper()
	engine := &siblingResultEngine{fakeEngine{date: retrieval.Date{Year: 2026, Month: 11, Day: 9}}}
	arms, err := engine.Collect(context.Background(), retrieval.Prepared{Validation: retrieval.Validation{Accepted: true}}, retrieval.Depths{Dense: 200, Lexical: 100})
	if err != nil {
		t.Fatal(err)
	}
	members := append([]retrieval.Candidate(nil), arms.Primary.Members...)
	article := members[0].Ancestors[0]
	article.Text = "lead"
	article.NodeID = "article"
	for i, text := range []string{"first clause", "second clause"} {
		c := &members[i]
		p := c.Descendants[0]
		p.Text = text
		c.Content = "lead\n" + text
		c.Ancestors = []retrieval.Provision{article, p}
		c.Descendants = []retrieval.Provision{p}
		c.Spans = []byte(fmt.Sprintf(`[{"node_id":"article","start":0,"end":4,"content_start":0,"content_end":4,"role":"lead"},{"node_id":%q,"start":0,"end":%d,"content_start":5,"content_end":%d,"role":"body"}]`, p.NodeID, len([]rune(text)), 5+len([]rune(text))))
	}
	arms.Primary = retrieval.ArmGroup{Dense: members, Lexical: members, Members: members}
	merge, err := engine.HydrateSiblingMerge(context.Background(), arms)
	if err != nil {
		t.Fatal(err)
	}
	recipe := engine.Release().RetrievalRecipe
	recipe.EvidenceRuneCap = 20
	return arms, recipe, merge
}

func TestMergeSiblings_ClippedMemberLossIsGrouping(t *testing.T) {
	for _, nonCurrent := range []bool{false, true} {
		t.Run(fmt.Sprintf("non_current_%t", nonCurrent), func(t *testing.T) {
			arms, recipe, merge := siblingLossFixture(t)
			if nonCurrent {
				arms.NonCurrent = arms.Primary
				arms.Primary = retrieval.ArmGroup{}
			}
			response, err := merge.Shape(arms, recipe, 8, "hybrid", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			hits := append(append([]retrieval.Hit(nil), response.Hits...), response.NonCurrent...)
			if len(hits) != 1 || hits[0].Text != "lead\nfirst clause\nse" {
				t.Fatalf("clipping fixture %+v", hits)
			}
			anchor := "second"
			entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "doc", Anchor: &anchor, Kind: "provision"}}}
			if MatchProduct(entry, hits) != 0 {
				t.Fatal("clipped clause received credit")
			}
			losses, _, err := caseDiagnostics(arms, recipe, 8, "hybrid", "old_product", []Expectation{{DocNumber: "doc"}}, map[int]MapEntry{0: entry}, response, nil, merge)
			if err != nil {
				t.Fatal(err)
			}
			if len(losses) != 1 || losses[0].Stage != "grouping" {
				t.Fatalf("losses %+v, want grouping", losses)
			}
		})
	}
}

func TestMergeSiblings_LateGroupLossRemainsRankLimit(t *testing.T) {
	arms, recipe, merge := siblingLossFixture(t)
	late := lossCandidate(10, "other", "late")
	arms.Primary.Dense = append(arms.Primary.Dense, late)
	arms.Primary.Lexical = append(arms.Primary.Lexical, late)
	arms.Primary.Members = append(arms.Primary.Members, late)
	recipe.NonCurrentTail = 0
	response, err := merge.Shape(arms, recipe, 1, "hybrid", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Hits) != 1 || response.Hits[0].Identity.Anchor != "article" {
		t.Fatalf("ranking fixture %+v", response)
	}
	anchor := "late"
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "other", Anchor: &anchor, Kind: "provision"}}}
	stage, err := diagnoseLoss(arms, recipe, 1, "hybrid", "old_product", Expectation{}, entry, response, nil, merge)
	if err != nil {
		t.Fatal(err)
	}
	if stage != "rank_limit" {
		t.Fatalf("stage %q, want rank_limit", stage)
	}
}
