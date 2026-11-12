package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"danny.vn/trachuan/internal/retrieval"
)

func lossCandidate(id int64, doc, anchor string) retrieval.Candidate {
	identity := retrieval.Identity{Country: "vn", DocumentKey: doc, Anchor: anchor}
	p := retrieval.Provision{ID: id, Identity: identity, NodeID: fmt.Sprint(id), Kind: "khoan", Label: "Khoản 1", Text: "text"}
	return retrieval.Candidate{ChunkID: id, Identity: identity, Group: identity, GroupID: id, ProvisionID: id, Cosine: .9, Document: retrieval.Document{Number: doc, StatusClass: "in_force", IsCurrent: true}, Provision: p, Ancestors: []retrieval.Provision{p}, Descendants: []retrieval.Provision{p}}
}
func TestLossDiagnostics_Stages(t *testing.T) {
	recipe := retrieval.Recipe1()
	threshold := 0.0
	recipe.Abstention.Threshold = &threshold
	recipe.DenseDepth = 2
	recipe.LexicalDepth = 2
	recipe.DocumentCap = 1
	a := lossCandidate(1, "doc", "a")
	b := lossCandidate(2, "doc", "b")
	other := lossCandidate(3, "other", "c")
	tests := []struct {
		name     string
		dense    []retrieval.Candidate
		recipe   retrieval.Recipe
		expected retrieval.Candidate
		limit    int
		abstain  bool
		want     string
	}{
		{"candidate", []retrieval.Candidate{a}, recipe, b, 2, false, "candidate"},
		{"fusion", []retrieval.Candidate{a, other, b}, recipe, b, 2, false, "fusion"},
		{"grouping", []retrieval.Candidate{a, b}, recipe, b, 2, false, "grouping"},
		{"cap", []retrieval.Candidate{a, b, other}, recipe, b, 2, false, "cap"},
		{"abstention", []retrieval.Candidate{a}, recipe, a, 2, true, "abstention"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := tt.recipe
			dense := append([]retrieval.Candidate(nil), tt.dense...)
			if tt.name == "cap" {
				rr.DenseDepth = 3
			}
			if tt.name == "grouping" {
				dense[1].GroupID = dense[0].GroupID
				dense[1].Group = dense[0].Group
				dense[1].Descendants = dense[0].Descendants
			}
			if tt.abstain {
				v := .95
				rr.Abstention.Threshold = &v
			}
			arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Date: retrieval.Date{Year: 2026, Month: time.November, Day: 9}, Primary: retrieval.ArmGroup{Dense: dense, Members: dense}}
			response, err := retrieval.Shape(arms, rr, tt.limit, "hybrid")
			if err != nil {
				t.Fatal(err)
			}
			anchor := tt.expected.Identity.Anchor
			entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: tt.expected.Identity.DocumentKey, Anchor: &anchor, Kind: "provision"}}}
			got, err := diagnoseLoss(arms, rr, tt.limit, "hybrid", "old_product", Expectation{}, entry, response, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("stage = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCaseLosses_StoredWithHitGaps(t *testing.T) {
	c := lossCandidate(1, "doc", "one")
	c.Document.StatusClass = "unknown"
	c.Document.IsCurrent = false
	recipe := retrieval.Recipe1()
	threshold := .95
	recipe.Abstention.Threshold = &threshold
	arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Date: retrieval.Date{Year: 2026, Month: time.November, Day: 9}, Primary: retrieval.ArmGroup{Dense: []retrieval.Candidate{c}, Members: []retrieval.Candidate{c}}}
	response, err := retrieval.Shape(arms, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	anchor := "one"
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "doc", Anchor: &anchor, Kind: "provision"}}}
	losses, gaps, err := caseDiagnostics(arms, recipe, 8, "hybrid", "old_product", []Expectation{{DocNumber: "doc"}}, map[int]MapEntry{0: entry}, response, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(losses) != 1 || losses[0].Stage != "abstention" || len(gaps) == 0 {
		t.Fatalf("losses %v gaps %v", losses, gaps)
	}
}

func TestLossDiagnostics_RelationCandidateUsesEvidenceHash(t *testing.T) {
	c := lossCandidate(1, "doc", "one")
	target := "target"
	c.Relations = []retrieval.SourceRelation{{Value: retrieval.Relation{SourceKey: "doc", TargetKey: &target, Kind: "amends", Evidence: "evidence"}}}
	sum := sha256.Sum256([]byte("evidence"))
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "doc", Kind: "relation", SourceKey: "doc", TargetKey: "target", RelationKind: "amends", EvidenceSHA256: hex.EncodeToString(sum[:])}}}
	if !matchingCandidate("old_product", Expectation{}, entry, []retrieval.Candidate{c}) {
		t.Fatal("relation candidate was missed")
	}
}

func TestLossDiagnostics_CapDemotesReturnedCitation(t *testing.T) {
	recipe := retrieval.Recipe1()
	threshold := 0.0
	recipe.Abstention.Threshold = &threshold
	recipe.DenseDepth = 3
	recipe.DocumentCap = 1
	a, b, c := lossCandidate(1, "doc", "a"), lossCandidate(2, "doc", "b"), lossCandidate(3, "other", "c")
	members := []retrieval.Candidate{a, b, c}
	arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Date: retrieval.Date{Year: 2026, Month: time.November, Day: 9}, Primary: retrieval.ArmGroup{Dense: members, Members: members}}
	response, err := retrieval.Shape(arms, recipe, 3, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	anchor := "b"
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "doc", Anchor: &anchor, Kind: "provision"}}}
	if rank := MatchProduct(entry, response.Hits); rank != 3 {
		t.Fatalf("rank = %d", rank)
	}
	stage, err := diagnoseLoss(arms, recipe, 3, "hybrid", "old_product", Expectation{}, entry, response, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stage != "cap" {
		t.Fatalf("stage = %s", stage)
	}
}

func TestLossDiagnostics_RecordsLateNaturalRank(t *testing.T) {
	recipe := retrieval.Recipe1()
	threshold := 0.0
	recipe.Abstention.Threshold = &threshold
	a, b := lossCandidate(1, "first", "a"), lossCandidate(2, "second", "b")
	members := []retrieval.Candidate{a, b}
	arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Date: retrieval.Date{Year: 2026, Month: time.November, Day: 9}, Primary: retrieval.ArmGroup{Dense: members, Members: members}}
	response, err := retrieval.Shape(arms, recipe, 8, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	anchor := "b"
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "second", Anchor: &anchor, Kind: "provision"}}}
	stage, err := diagnoseLoss(arms, recipe, 8, "hybrid", "old_product", Expectation{}, entry, response, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stage != "ranked" {
		t.Fatalf("stage = %q", stage)
	}
}

func TestLossDiagnostics_FirstAlternativeAcrossGroups(t *testing.T) {
	recipe := retrieval.Recipe1()
	threshold := 0.0
	recipe.Abstention.Threshold = &threshold
	recipe.DocumentCap = 1
	primary := []retrieval.Candidate{lossCandidate(1, "primary-first", "a"), lossCandidate(2, "primary-target", "b")}
	other := []retrieval.Candidate{lossCandidate(3, "historic-target", "a"), lossCandidate(4, "historic-target", "b"), lossCandidate(5, "historic-other", "a")}
	arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Primary: retrieval.ArmGroup{Dense: primary, Members: primary}, NonCurrent: retrieval.ArmGroup{Dense: other, Members: other}, IncludeNonCurrent: true}
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "primary-target", Kind: "document"}, {Country: "vn", DocumentKey: "historic-target", Anchor: &other[1].Identity.Anchor, Kind: "provision"}}}
	for _, useTail := range []bool{false, true} {
		t.Run(fmt.Sprintf("tail_%t", useTail), func(t *testing.T) {
			var response retrieval.Response
			var tail *int
			var err error
			if useTail {
				v := 3
				tail = &v
				response, err = retrieval.ShapeWithNonCurrentTail(arms, recipe, 5, "dense", nil, v)
			} else {
				response, err = retrieval.Shape(arms, recipe, 5, "dense")
			}
			if err != nil {
				t.Fatal(err)
			}
			if diagnosticRank("old_product", Expectation{}, entry, response.Hits) != 2 || diagnosticRank("old_product", Expectation{}, entry, response.NonCurrent) != 3 {
				t.Fatalf("unexpected response: %+v", response)
			}
			stage, err := diagnoseLoss(arms, recipe, 5, "dense", "old_product", Expectation{}, entry, response, tail)
			if err != nil {
				t.Fatal(err)
			}
			if stage != "ranked" {
				t.Fatalf("stage = %q, want ranked", stage)
			}
		})
	}
}

func TestLossDiagnostics_TailReservationIsRankLimit(t *testing.T) {
	tailLossTest(t, false)
}

func TestLossDiagnostics_ComplementBeyondTailIsRankLimit(t *testing.T) {
	tailLossTest(t, true)
}

func TestLossDiagnostics_HistoricalComplementBeyondLimit(t *testing.T) {
	recipe := retrieval.Recipe1()
	threshold := 0.0
	recipe.Abstention.Threshold = &threshold
	primary := []retrieval.Candidate{lossCandidate(1, "a", "a"), lossCandidate(2, "b", "b")}
	other := lossCandidate(3, "c", "c")
	arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Primary: retrieval.ArmGroup{Dense: primary, Members: primary}, NonCurrent: retrieval.ArmGroup{Dense: []retrieval.Candidate{other}, Members: []retrieval.Candidate{other}}, IncludeNonCurrent: true}
	response, err := retrieval.Shape(arms, recipe, 2, "dense")
	if err != nil {
		t.Fatal(err)
	}
	anchor := "c"
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "c", Anchor: &anchor, Kind: "provision"}}}
	stage, err := diagnoseLoss(arms, recipe, 2, "dense", "old_product", Expectation{}, entry, response, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stage != "rank_limit" {
		t.Fatalf("stage = %q, want rank_limit", stage)
	}
}

func tailLossTest(t *testing.T, complement bool) {
	t.Helper()
	recipe := retrieval.Recipe1()
	threshold := 0.0
	recipe.Abstention.Threshold = &threshold
	primary := []retrieval.Candidate{lossCandidate(1, "a", "a"), lossCandidate(2, "b", "b"), lossCandidate(3, "c", "c")}
	other := []retrieval.Candidate{lossCandidate(4, "d", "d"), lossCandidate(5, "e", "e")}
	arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Primary: retrieval.ArmGroup{Dense: primary, Members: primary}, NonCurrent: retrieval.ArmGroup{Dense: other, Members: other}, IncludeNonCurrent: true}
	response, err := retrieval.ShapeWithNonCurrentTail(arms, recipe, 3, "dense", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	target := primary[2]
	if complement {
		target = other[1]
	}
	anchor := target.Identity.Anchor
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: target.Identity.DocumentKey, Anchor: &anchor, Kind: "provision"}}}
	tail := 1
	stage, err := diagnoseLoss(arms, recipe, 3, "dense", "old_product", Expectation{}, entry, response, &tail)
	if err != nil {
		t.Fatal(err)
	}
	if stage != "rank_limit" {
		t.Fatalf("stage = %q, want rank_limit", stage)
	}
}

func TestLossDiagnostics_DefaultTailIncludesComplement(t *testing.T) {
	recipe := retrieval.Recipe1()
	threshold := 0.0
	recipe.Abstention.Threshold = &threshold
	primary := []retrieval.Candidate{lossCandidate(1, "a", "a"), lossCandidate(2, "b", "b"), lossCandidate(3, "c", "c")}
	other := []retrieval.Candidate{lossCandidate(4, "d", "d"), lossCandidate(5, "e", "e"), lossCandidate(6, "f", "f")}
	arms := retrieval.Arms{Validation: retrieval.Validation{Accepted: true}, Depths: retrieval.Depths{Dense: 200, Lexical: 100}, Primary: retrieval.ArmGroup{Dense: primary, Members: primary}, NonCurrent: retrieval.ArmGroup{Dense: other, Members: other}}
	response, err := retrieval.Shape(arms, recipe, 3, "dense")
	if err != nil {
		t.Fatal(err)
	}
	anchor := "f"
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Country: "vn", DocumentKey: "f", Anchor: &anchor, Kind: "provision"}}}
	stage, err := diagnoseLoss(arms, recipe, 3, "dense", "old_product", Expectation{}, entry, response, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stage != "rank_limit" {
		t.Fatalf("stage = %q, want rank_limit", stage)
	}
}
