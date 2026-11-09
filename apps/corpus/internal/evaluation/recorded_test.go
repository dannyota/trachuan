package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

type recordedEngine struct {
	fakeEngine
	fallback bool
}

func (f *recordedEngine) Collect(_ context.Context, p retrieval.Prepared, d retrieval.Depths) (retrieval.Arms, error) {
	out := retrieval.Arms{Validation: p.Validation, Depths: d, Date: f.date}
	if p.Request.Query != "Câu hỏi tốt" {
		return out, nil
	}
	hit := lossCandidate(1, "doc", "one")
	out.Primary.Dense = []retrieval.Candidate{hit}
	out.Primary.Lexical = []retrieval.Candidate{hit}
	out.Primary.Members = []retrieval.Candidate{hit}
	if p.Request.Mode == "ann" {
		if f.fallback {
			raw := lossCandidate(2, "raw", "other")
			out.Primary.RawANN = []retrieval.Candidate{raw}
			out.Primary.Fallback = true
		} else {
			out.Primary.RawANN = []retrieval.Candidate{hit}
		}
	}
	return out, nil
}
func recordedSuite() Suite {
	cases := []Case{{ID: "positive", Question: "Câu hỏi tốt", ExpectedCitations: []Expectation{{DocNumber: "doc"}}}, {ID: "negative", Question: "Câu hỏi không", ExpectAbstain: true}, {ID: "reject1", Question: "khong dau mot", ExpectedCitations: []Expectation{{DocNumber: "doc"}}}, {ID: "reject2", Question: "khong dau hai", ExpectedCitations: []Expectation{{DocNumber: "doc"}}}, {ID: "reject3", Question: "khong dau ba", ExpectedCitations: []Expectation{{DocNumber: "doc"}}}}
	entries := []MapEntry{}
	for _, c := range cases {
		if len(c.ExpectedCitations) == 0 {
			continue
		}
		entries = append(entries, MapEntry{CaseID: c.ID, CitationIndex: 0, State: "resolved", ExpectedNumber: "doc", Reason: "reviewed", ReviewedAt: "2026-11-09", Evidence: []Evidence{{FetchSHA256: "hash", QuotedSpan: "evidence"}}, Alternatives: []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "document"}}})
	}
	return Suite{Legacy: cases, Rejections: []string{"reject1", "reject2", "reject3"}, Hashes: FixtureHashes{Legacy: "fixture", Inventory: "inventory"}, Map: IdentityMap{Version: 1, LegacySHA256: "fixture", IdentityInventoryHash: "inventory", Entries: entries}}
}
func TestRun_RecordedArmsPassQuality(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	f := &recordedEngine{fakeEngine: fakeEngine{date: date}}
	r, _, err := Run(context.Background(), f, recordedSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "complete" || !r.QualityPass || r.ReleaseEligible {
		t.Fatalf("status %s quality %v release %v", r.Status, r.QualityPass, r.ReleaseEligible)
	}
	for _, floor := range r.Floors {
		if floor.Status != "pass" {
			t.Fatalf("floor %s %s: %s", floor.Mode, floor.Name, floor.Status)
		}
	}
}
func TestRun_RecordedANNFallback(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	f := &recordedEngine{fakeEngine: fakeEngine{date: date}, fallback: true}
	_, cache, err := Run(context.Background(), f, recordedSuite(), Options{EvaluationDate: date})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = cache.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(dir, "arms.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var a ArmRow
		if err = decoder.Decode(&a); err != nil {
			t.Fatal(err)
		}
		if a.CaseID == "positive" && a.Cohort == "old_product" && a.Mode == "ann" {
			if !a.Fallback || len(a.RawANN) != 1 || len(a.Dense) != 1 || a.RawANN[0].ChunkID == a.Dense[0].ChunkID {
				t.Fatalf("invalid fallback cache: %+v", a)
			}
			return
		}
	}
	t.Fatal("missing ANN cache")
}

func TestRun_CaseJSONIncludesLosses(t *testing.T) {
	date := retrieval.Date{Year: 2026, Month: time.November, Day: 9}
	f := &fakeEngine{date: date}
	report, _, err := Run(context.Background(), f, recordedSuite(), Options{EvaluationDate: date, Modes: []retrieval.Mode{"exact"}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = report.WriteFiles(dir); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(dir, "cases.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	dec := json.NewDecoder(file)
	for dec.More() {
		var row CaseResult
		if err = dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.CaseID == "positive" && row.Cohort == "old_product" && row.Mode == "exact" && row.Arms == "hybrid" && row.Limit == 8 {
			if len(row.Losses) != 1 || row.Losses[0].Stage != "candidate" {
				t.Fatalf("losses %v", row.Losses)
			}
			return
		}
	}
	t.Fatal("case row missing")
}
