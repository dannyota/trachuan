package discover

import (
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestCeiling_RunWideOverflowFailsDiscover(t *testing.T) {
	observations := []secondaryObservation{
		{key: "chinhphu:keyword:a", hash: "one", report: secondaryReport{Candidates: []secondaryCandidate{{ID: "1"}, {ID: "2"}}}},
		{key: "chinhphu:keyword:b", hash: "two", report: secondaryReport{Candidates: []secondaryCandidate{{ID: "2"}, {ID: "3"}}}},
	}
	failure := secondaryCeiling(observations, 2)
	if failure.Code != "secondary_candidate_ceiling" || !strings.Contains(failure.Reason, "3") || !strings.Contains(failure.Reason, "2") || len(failure.EvidenceRefs) != 2 {
		t.Fatalf("failure=%+v", failure)
	}
}

func TestSupport_PerQueryTuplesSurviveSupersession(t *testing.T) {
	old := membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:b|1"}
	records := []membership.EvidenceRecord{{State: "current", Support: membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:a|1"}}}
	observed := map[string]secondaryObservation{"chinhphu:keyword:b": {key: "chinhphu:keyword:b", report: secondaryReport{Stable: true, Candidates: []secondaryCandidate{{ID: "1", State: "incomplete"}}}}}
	if superseded(old, records) || secondaryPriorState(old, observed) != "retained" {
		t.Fatal("incomplete independent query lost support")
	}
	prior := []membership.Entry{{DocumentKey: "vn:chinhphu:1", Source: "chinhphu", SourceID: "1", Class: "secondary", Disposition: "active", Supports: []membership.SupportTuple{old}}}
	members := map[string]*seedMember{}
	retainSecondary(prior, observed, members)
	if got := members["vn:chinhphu:1"]; got == nil || len(got.records) != 1 || got.records[0].State != "retained" {
		t.Fatalf("member=%+v", got)
	}
}

func TestSupport_ChangedQueryIdentityRemovesOldSupport(t *testing.T) {
	oldID := "old"
	old := membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:a|1", Evidence: membership.EvidenceRef{QueryIdentity: &oldID}}
	observations := map[string]secondaryObservation{"chinhphu:keyword:a": {report: secondaryReport{QueryIdentity: "new", Stable: false}}}
	if got := secondaryPriorState(old, observations); got != "removed" {
		t.Fatalf("state=%s", got)
	}
}

func TestSupport_FirstCompleteAbsenceRetains(t *testing.T) {
	prior := []membership.Entry{{DocumentKey: "vn:chinhphu:1", Source: "chinhphu", SourceID: "1", Class: "secondary", Disposition: "active", Supports: []membership.SupportTuple{{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:a|1"}}}}
	obs := map[string]secondaryObservation{"chinhphu:keyword:a": {report: secondaryReport{Stable: true}}}
	members := map[string]*seedMember{}
	retainSecondary(prior, obs, members)
	m := members["vn:chinhphu:1"]
	if m.entry.Disposition != "active" || m.records[0].State != "retained" {
		t.Fatalf("member=%+v", m)
	}
}

func TestSupport_ConflictingDetailsRetainPrior(t *testing.T) {
	obs := []secondaryObservation{
		{key: "chinhphu:keyword:a", report: secondaryReport{Stable: true, Candidates: []secondaryCandidate{{ID: "1", State: "admit_secondary", Number: "142/2026/NĐ-CP", Issuer: "central", IssuerForm: "Chính phủ", Date: "2026-04-30"}}}},
		{key: "chinhphu:keyword:b", report: secondaryReport{Stable: true, Candidates: []secondaryCandidate{{ID: "1", State: "admit_secondary", Number: "142/2026/NĐ-CP", Issuer: "central", IssuerForm: "Chính phủ", Date: "2026-05-01"}}}},
	}
	markSecondaryConflicts(obs)
	prior := membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:a|1"}
	if got := secondaryPriorState(prior, secondaryObservationMap(obs)); got != "retained" {
		t.Fatalf("state=%s", got)
	}
}

func TestSupport_StableAbsenceUsesLastPass(t *testing.T) {
	id := "1"
	obs := map[string]secondaryObservation{"chinhphu:keyword:a": {report: secondaryReport{Stable: true, Passes: []lookup.Result{{Items: []lookup.Item{{SourceID: id}}}, {Items: []lookup.Item{}}, {Items: []lookup.Item{}}}}}}
	prior := []membership.Entry{{DocumentKey: "vn:chinhphu:1", Source: "chinhphu", SourceID: id, Class: "secondary", Disposition: "active", Supports: []membership.SupportTuple{{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:a|1"}}}}
	members := map[string]*seedMember{}
	retainSecondary(prior, obs, members, map[string]secondaryHistory{"chinhphu:keyword:a": {present: map[string]bool{}}})
	if got := members["vn:chinhphu:1"].entry.Disposition; got != "withdrawn" {
		t.Fatalf("disposition=%s", got)
	}
}

func TestSupport_ProvenReplacementKeepsSecondaryForFetch(t *testing.T) {
	old := membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:a|1"}
	prior := []membership.Entry{{DocumentKey: "vn:chinhphu:1", Source: "chinhphu", SourceID: "1", Class: "secondary", Disposition: "active", Supports: []membership.SupportTuple{old}}}
	obs := map[string]secondaryObservation{"chinhphu:keyword:a": {report: secondaryReport{Stable: true, Passes: []lookup.Result{{Items: []lookup.Item{{SourceID: "1"}}}}, Candidates: []secondaryCandidate{{ID: "1", State: "seed_primary", PrimaryID: "v1"}}}}}
	current := old
	current.Evidence.ResponseSHA256 = "current"
	members := map[string]*seedMember{"vn:chinhphu:1": {entry: prior[0], records: []membership.EvidenceRecord{{DocumentKey: "vn:chinhphu:1", Support: current, State: "current"}}}}
	retainSecondary(prior, obs, members)
	m := members["vn:chinhphu:1"]
	if m.entry.Disposition != "active" || len(m.records) != 1 || m.records[0].State != "current" {
		t.Fatal("current secondary support was not preserved")
	}
}

func TestSupport_ProbeRetentionUsesCompoundItemKey(t *testing.T) {
	s := membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: "chinhphu:keyword:a|1"}
	if got := priorEvidenceItem(s); got != "chinhphu:keyword:a" {
		t.Fatalf("item key=%s", got)
	}
}
