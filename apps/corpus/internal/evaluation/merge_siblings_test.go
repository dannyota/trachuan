package evaluation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
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
	const want = "16597abd1cf72fb321c6c52b1ccc8fd24bb11b3e56065cd7bbdb45d0ed26c7df"
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != want {
		t.Fatalf("default report sha256 = %s", got)
	}
}

type mergeEngine struct {
	fakeEngine
	hydrations int
}

func (e *mergeEngine) HydrateSiblingMerge(_ context.Context, _ retrieval.Arms) (*retrieval.SiblingMerge, error) {
	e.hydrations++
	return &retrieval.SiblingMerge{}, nil
}

func TestMergeSiblings_ReportRecordsAndFilterGate(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	for _, enabled := range []bool{false, true} {
		e := &mergeEngine{fakeEngine: fakeEngine{date: date}}
		r, _, err := Run(context.Background(), e, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}, MergeSiblings: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if (e.hydrations > 0) != enabled {
			t.Fatalf("hydrations %d enabled %t", e.hydrations, enabled)
		}
		if enabled && (r.FilterGate.Reason != "research_override" || r.FilterGate.Modes[0].Status != "not_run") {
			t.Fatalf("gate %+v", r.FilterGate)
		}
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var root any
		if err = json.Unmarshal(b, &root); err != nil {
			t.Fatal(err)
		}
		var visit func(any)
		visit = func(value any) {
			switch v := value.(type) {
			case []any:
				for _, item := range v {
					visit(item)
				}
			case map[string]any:
				_, isReport := v["scorer"]
				_, hasLimit := v["limit"]
				if isReport || hasLimit {
					got, present := v["merge_siblings"]
					if present != enabled || (enabled && got != true) {
						t.Errorf("override %v present %v enabled %v", got, present, enabled)
					}
				}
				for _, item := range v {
					visit(item)
				}
			}
		}
		visit(root)
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

func TestMergeSiblings_PreviousDiffAndCaseFiles(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	baseline, _, err := Run(context.Background(), &fakeEngine{date: date}, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = baseline.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	r, _, err := Run(context.Background(), &mergeEngine{fakeEngine: fakeEngine{date: date}}, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}, MergeSiblings: true, PreviousReport: filepath.Join(dir, "report.json")})
	if err != nil {
		t.Fatal(err)
	}
	diff := r.PreviousDiff.(map[string]any)
	if diff["reason"] != "incompatible sibling merge override" {
		t.Fatalf("diff %+v", diff)
	}
	prior := r.Cases[0]
	prior.Abstain = !prior.Abstain
	differences := compareCases([]CaseResult{prior}, r.Cases[:1])
	if len(differences) != 1 || !differences[0].MergeSiblings {
		t.Fatalf("differences %+v", differences)
	}
	dir = t.TempDir()
	if err = r.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "cases.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
		var row map[string]any
		if err = json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		if row["merge_siblings"] != true {
			t.Fatal("case file omitted override")
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
	return retrieval.Arms{Validation: p.Validation, Depths: d, Date: e.date, Primary: retrieval.ArmGroup{Dense: members, Lexical: members, Members: members}}, nil
}

func (e *siblingResultEngine) HydrateSiblingMerge(_ context.Context, arms retrieval.Arms) (*retrieval.SiblingMerge, error) {
	first := arms.Primary.Dense[0]
	article := first.Ancestors[0]
	nodes := []retrieval.Provision{article}
	for _, c := range arms.Primary.Dense {
		nodes = append(nodes, c.Descendants...)
	}
	return &retrieval.SiblingMerge{Articles: map[int64]retrieval.Candidate{1: {GroupID: 1, Group: article.Identity, Identity: article.Identity, Ancestors: []retrieval.Provision{article}, Descendants: nodes, Document: first.Document}}}, nil
}

func TestMergeSiblings_EvaluationScoresMergedClauses(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	suite := recordedSuite()
	anchor := "second"
	suite.Map.Entries[0].Alternatives = []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "provision", Anchor: &anchor}}
	engine := &siblingResultEngine{fakeEngine{date: date}}
	hash := engine.Release().RetrievalRecipe.Hash()
	r, cache, err := Run(context.Background(), engine, suite, Options{EvaluationDate: date, MergeSiblings: true})
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
			if g.Identity.Anchor != "first" && g.Identity.Anchor != "second" {
				t.Fatalf("cached override group %+v", g)
			}
		}
	}
	if engine.Release().RetrievalRecipe.Hash() != hash {
		t.Fatal("research changed recipe")
	}
}

func TestMergeSiblings_CacheByteParity(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: 11, Day: 9}
	var baseline Cache
	var baselineKey any
	var baselineDir string
	for _, enabled := range []bool{false, true} {
		engine := &siblingResultEngine{fakeEngine{date: date}}
		report, cache, err := Run(context.Background(), engine, recordedSuite(), Options{EvaluationDate: date, MergeSiblings: enabled})
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err = cache.WriteFiles(dir); err != nil {
			t.Fatal(err)
		}
		if !enabled {
			baseline = cache
			baselineKey = report.CacheManifest["cache_key"]
			baselineDir = dir
			continue
		}
		if report.CacheManifest["cache_key"] != baselineKey {
			t.Fatal("merge changed candidate cache key")
		}
		before, err := json.Marshal(baseline)
		if err != nil {
			t.Fatal(err)
		}
		after, err := json.Marshal(cache)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Error("merge changed serialized candidate cache")
		}
		for _, name := range []string{"arms.jsonl", "query-index.json", "queries.f32"} {
			before, err = os.ReadFile(filepath.Join(baselineDir, name))
			if err != nil {
				t.Fatal(err)
			}
			after, err = os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("merge changed cache file %s", name)
			}
		}
	}
}
