//go:build integration

package retrieval

import (
	"context"
	"testing"
	"time"
)

func TestCollect_ValidityConflictNotInPrimaryGroup(t *testing.T) {
	db, e, _ := fixture(t)
	if _, err := db.Exec(context.Background(), `UPDATE document SET validity_conflict=true WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	no := false
	for _, mode := range []Mode{"exact", "ann"} {
		a := collectFixture(t, e, Request{Query: "ngân hàng", Mode: mode, InForceOnly: &no}, Depths{10, 10})
		for _, arm := range [][]Candidate{a.Primary.Dense, a.Primary.Lexical, a.Primary.Members, a.Primary.RawANN} {
			for _, c := range arm {
				if c.Identity.DocumentKey == "vn:1" {
					t.Fatalf("conflict in primary %s", mode)
				}
			}
		}
		for name, arm := range map[string][]Candidate{"dense": a.NonCurrent.Dense, "lexical": a.NonCurrent.Lexical} {
			found := false
			for _, c := range arm {
				if c.Identity.DocumentKey == "vn:1" {
					found = true
					validity, _ := stamp(c.Document, a)
					if validity.Current || !validity.Unresolved {
						t.Fatalf("%s %s conflict stamp: %+v", mode, name, validity)
					}
				}
			}
			if !found {
				t.Fatalf("conflict missing from %s non-current %s", name, mode)
			}
		}

		a = collectFixture(t, e, Request{Query: "ngân hàng", Mode: mode, AsOf: &Date{2021, time.January, 1}}, Depths{10, 10})
		found := false
		for _, c := range a.Primary.Dense {
			if c.Identity.DocumentKey == "vn:1" {
				found = true
			}
		}
		if !found {
			t.Fatalf("conflict missing from dated results %s", mode)
		}
	}
}

func TestHit_CarriesClassSourceAndDecisions(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `UPDATE document SET class='relation_context',source='chinhphu',validity_conflict=true WHERE id=1;
 INSERT INTO validity_decision(document_id,ordinal,layer,rule_version,before,after) VALUES (1,2,'manual','2','{}','{}'),(1,1,'source','1','{}','{}')`)
	if err != nil {
		t.Fatal(err)
	}
	no := false
	for _, date := range []*Date{nil, {2021, time.January, 1}} {
		result, err := e.Search(ctx, Request{Query: "ngân hàng", Mode: "exact", InForceOnly: &no, AsOf: date})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, hits := range [][]Hit{result.Hits, result.NonCurrent} {
			for _, hit := range hits {
				if hit.Identity.DocumentKey == "vn:1" {
					found = true
					if hit.Class != "relation_context" || hit.Source != "chinhphu" || len(hit.Stamp.Decisions) != 2 || hit.Stamp.Decisions[0].Ordinal != 1 || hit.Stamp.Decisions[1].Ordinal != 2 || hit.Stamp.Unresolved != (date == nil) {
						t.Fatalf("hit: %+v", hit)
					}
				}
			}
		}
		if !found {
			t.Fatal("missing hit")
		}
	}
}

func TestLoadDecisions_ShapedHits(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO validity_decision(document_id,ordinal,layer,rule_version,before,after) VALUES (1,1,'source','1','{}','{}')`); err != nil {
		t.Fatal(err)
	}
	recipe := e.Release().RetrievalRecipe
	arms := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact"}, Depths{recipe.DenseDepth, recipe.LexicalDepth})
	response, err := Shape(arms, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.LoadDecisions(ctx, response.Hits, response.NonCurrent); err != nil {
		t.Fatal(err)
	}
	for _, hit := range response.Hits {
		if hit.Identity.DocumentKey == "vn:1" {
			if len(hit.Stamp.Decisions) != 1 || hit.Stamp.Decisions[0].Layer != "source" {
				t.Fatalf("decisions: %+v", hit.Stamp.Decisions)
			}
			return
		}
	}
	t.Fatal("missing hit")
}
