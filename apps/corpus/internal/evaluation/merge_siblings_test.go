package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestMergeSiblings_DefaultSerialization(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	r, _, err := Run(context.Background(), &fakeEngine{date: date}, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	r.TimingsMS = nil
	r.CacheManifest = nil
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	const want = "65401f4c1b03db61e21aed2bd47504737485e667995d398b3e2678fd42d737d9"
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != want {
		t.Fatalf("default report sha256 = %s", got)
	}
}

func TestMergeSiblings_ProductCreditsOnlyCompleteMembers(t *testing.T) {
	id := retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "article"}
	hit := retrieval.Hit{Identity: id, Fragments: []retrieval.Fragment{{Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "first"}, Complete: true}, {Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "second"}, Complete: false}}}
	for _, tc := range []struct {
		anchor string
		rank   int
	}{{"first", 1}, {"second", 0}, {"other", 0}} {
		entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Kind: "provision", Country: "vn", DocumentKey: "doc", Anchor: &tc.anchor}}}
		if got := MatchProduct(entry, []retrieval.Hit{hit}); got != tc.rank {
			t.Fatalf("%s: %d", tc.anchor, got)
		}
	}
}

type siblingResultEngine struct{ fakeEngine }

func (e *siblingResultEngine) Collect(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	a := retrieval.Provision{ID: 1, Kind: "dieu", Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "article"}}
	members := []retrieval.Candidate{}
	for i, anchor := range []string{"first", "second"} {
		clause := retrieval.Provision{ID: int64(i + 2), ParentID: &a.ID, Kind: "khoan", Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: anchor}, NodeID: anchor, Text: anchor, Ordinal: i, Preorder: i + 1}
		members = append(members, retrieval.Candidate{ChunkID: clause.ID, GroupID: clause.ID, Group: clause.Identity, Identity: clause.Identity, Content: anchor, Ancestors: []retrieval.Provision{a, clause}, Descendants: []retrieval.Provision{clause}, Document: retrieval.Document{Number: "doc", IsCurrent: true, StatusClass: "in_force"}, Cosine: .7})
	}
	nodes := []retrieval.Provision{a}
	for _, c := range members {
		nodes = append(nodes, c.Descendants...)
	}
	articles := map[int64]retrieval.Candidate{1: {GroupID: 1, Group: a.Identity, Identity: a.Identity, Ancestors: []retrieval.Provision{a}, Descendants: nodes, Document: members[0].Document}}
	return retrieval.Arms{Articles: articles, Release: "20990101_1", Validation: p.Validation, Depths: d, Date: e.date, Primary: retrieval.ArmGroup{Dense: members, Lexical: members, Members: members}}, nil
}

func (e *siblingResultEngine) Release() retrieval.Release {
	r := e.fakeEngine.Release()
	r.RetrievalRecipe.Version = 3
	r.RetrievalRecipe.SiblingMerge = "article-clauses/1"
	return r
}
func TestMergeSiblings_EvaluationScoresMergedClauses(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	suite := recordedSuite()
	anchor := "second"
	suite.Map.Entries[0].Alternatives = []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "provision", Anchor: &anchor}}
	engine := &siblingResultEngine{fakeEngine{date: date}}
	hash := engine.Release().RetrievalRecipe.Hash()
	r, cache, err := Run(context.Background(), engine, suite, Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	for _, c := range r.Cases {
		if c.CaseID == "positive" && c.Cohort == "old_product" {
			checked = true
			if len(c.Returned) != 1 || c.Returned[0].Anchor != "article" || len(c.Ranks) != 1 || c.Ranks[0] != 1 {
				t.Fatalf("scored case %+v", c)
			}
		}
	}
	if !checked {
		t.Fatal("missing scored case")
	}
	for _, a := range cache.Arms {
		for _, g := range a.Groups {
			if g.Identity.Anchor != "article" {
				t.Fatalf("cached recipe group %+v", g)
			}
		}
	}
	if engine.Release().RetrievalRecipe.Hash() != hash {
		t.Fatal("evaluation changed recipe")
	}
}

func TestRecipe3_PreviousRecipe2Comparable(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	baseline, _, err := Run(context.Background(), &fakeEngine{date: date}, recordedSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = baseline.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	current, _, err := Run(context.Background(), &siblingResultEngine{fakeEngine{date: date}}, recordedSuite(), Options{EvaluationDate: date, PreviousReport: filepath.Join(dir, "report.json")})
	if err != nil {
		t.Fatal(err)
	}
	diff := current.PreviousDiff.(map[string]any)
	if diff["previous_recipe_version"] != 2 || diff["recipe_version"] != 3 || diff["cases"] == nil {
		t.Fatalf("comparison %+v", diff)
	}
	if len(current.NonRegression) == 0 {
		t.Fatal("missing non-regression")
	}
}
