package evaluation

import (
	"danny.vn/trachuan/internal/retrieval"
	"testing"
)

func TestReport_CountsByClassSourceAndBasis(t *testing.T) {
	r := Report{}
	r.addHits("case", []retrieval.Hit{{Class: "primary", Source: "vbpl", Stamp: retrieval.Validity{StatusBasis: "source_status"}}, {Class: "relation_context", Source: "other", Stamp: retrieval.Validity{StatusBasis: "derived_from_text", Unresolved: true}}}, nil)
	if r.HitsByClass["primary"] != 1 || r.HitsByClass["relation_context"] != 1 || r.HitsByStatusBasis["source_status"] != 1 || r.HitsByConflictKind["unresolved"] != 1 {
		t.Fatalf("unexpected counts: %+v", r)
	}
}

func TestValidityCases_ScoredOverPrimaryGroup(t *testing.T) {
	r := Report{}
	cases := []ValidityCase{{Number: "52/2024/NĐ-CP", Current: true}}
	r.addHits("case-1", []retrieval.Hit{{Number: "52/2024/NĐ-CP", Stamp: retrieval.Validity{Current: true}}, {Number: "52/2024/NĐ-CP"}}, cases)
	if r.ValidityAdjudicated.Hits != 2 || r.ValidityAdjudicated.Agree != 1 || r.ValidityAdjudicated.Disagree != 1 || len(r.ValidityAdjudicated.Disagreements) != 1 {
		t.Fatalf("unexpected score: %+v", r.ValidityAdjudicated)
	}
}

func TestValidityCases_AbsentDocumentNotEvaluated(t *testing.T) {
	r := Report{}
	r.addHits("case", nil, []ValidityCase{{Number: "absent", Current: true}})
	if r.ValidityAdjudicated.Agree != 0 || r.ValidityAdjudicated.Hits != 0 {
		t.Fatalf("absent counted: %+v", r.ValidityAdjudicated)
	}
}

func TestValidityCases_FixturePinnedByHash(t *testing.T) {
	s, err := VNSuite()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ValidityCases) != 5 || s.Hashes.ValidityCases != "3eae05af43d3c86eeb42bab1e95e4ea4241fbd656c533a3e0d54c95db8102822" {
		t.Fatal("missing validity fixture or hash")
	}
	s.ValidityBytes[0] ^= 1
	if err := s.VerifyValidityBytes(); err == nil {
		t.Fatal("changed fixture accepted")
	}
}

func TestMetrics_StatusConsistencyDenominatorUnchanged(t *testing.T) {
	m := Metrics{Cases: []CaseScore{{Negative: true, Response: retrieval.Response{Abstain: true, Hits: []retrieval.Hit{{Class: "relation_context", Stamp: retrieval.Validity{Current: true, StatusClass: "in_force"}}, {Class: "primary"}}, NonCurrent: []retrieval.Hit{{}}}}}}
	d := m.PrecisionCounts()
	if d.PrimaryTotal != 2 || d.PrimaryCurrent != 1 {
		t.Fatalf("counts: %+v", d)
	}
}
