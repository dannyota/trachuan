package fetch

import (
	"reflect"
	"sort"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
)

func TestHandOver_SourceReplaced(t *testing.T) {
	outcome := decideHandOver([]handOverPair{{key: "vn:vbpl:1", state: "match"}})
	if outcome.kind != "source_replaced" || !reflect.DeepEqual(outcome.keys, []string{"vn:vbpl:1"}) {
		t.Fatalf("outcome=%+v", outcome)
	}
}
func TestHandOver_AmbiguousKeepsBoth(t *testing.T) {
	for _, pairs := range [][]handOverPair{{{key: "vn:vbpl:1", state: "ambiguous"}}, {{key: "vn:vbpl:2", state: "match"}, {key: "vn:vbpl:1", state: "match"}}} {
		got := decideHandOver(pairs)
		if got.kind != "source_identity_ambiguous" {
			t.Fatalf("outcome=%+v", got)
		}
	}
}
func TestHandOver_DifferentDateKeepsBoth(t *testing.T) {
	if got := decideHandOver([]handOverPair{{key: "vn:vbpl:1", state: "different_date"}}); got.kind != "" {
		t.Fatalf("outcome=%+v", got)
	}
}
func TestHandOver_FailedFetchKeepsProvenMatch(t *testing.T) {
	secondary := handOverIdentity{number: "42", issuer: "A", date: "2026-01-01"}
	primary := handOverIdentity{number: "42", issuer: "A", date: "2026-01-01"}
	if got := compareHandOver(secondary, primary); got != "match" {
		t.Fatalf("comparison=%s", got)
	}
}
func TestHandOver_DiscoveryFetchConflictIsAmbiguous(t *testing.T) {
	a := handOverIdentity{number: "42", issuer: "A", date: "2026-01-01"}
	b := handOverIdentity{number: "42", issuer: "A", date: "2026-01-02"}
	if !identityConflict(a, b) {
		t.Fatal("missed conflicting date")
	}
}

func TestHandOver_ConflictsSurviveMissingIntermediateEvidence(t *testing.T) {
	e := handOverEvidence{pairs: map[string][]handOverObservation{}}
	for _, issuer := range []string{"A", "", "B"} {
		e.addRecord(handOverRecord{complete: true, record: discover.HandOverRecord{Number: "42", Issuer: issuer, Date: "2026-01-01"}}, nil)
	}
	if !e.conflict {
		t.Fatal("conflicting issuers were hidden by missing evidence")
	}
}

func TestHandOver_EvidenceOnlyIncludesComparedPairs(t *testing.T) {
	secondary := handOverIdentity{number: "42", issuer: "A", date: "2026-01-01"}
	evidence := handOverEvidence{secondary: secondary, hasSecondary: true, pairs: map[string][]handOverObservation{}, refs: []string{"secondary/document.json"}}
	primary := map[string]handOverFetched{
		"vn:vbpl:1": {done: true, identity: secondary, refs: []string{"match/detail.json"}},
		"vn:vbpl:2": {done: true, identity: handOverIdentity{number: "42", issuer: "A", date: "2026-01-02"}, refs: []string{"different-date/detail.json"}},
		"vn:vbpl:3": {done: true, identity: handOverIdentity{number: "43", issuer: "A", date: "2026-01-01"}, refs: []string{"unrelated-1/detail.json"}},
		"vn:vbpl:4": {done: true, identity: handOverIdentity{number: "44", issuer: "A", date: "2026-01-01"}, refs: []string{"unrelated-2/detail.json"}},
	}
	evidence.pairOutcomes(primary)
	sort.Strings(evidence.refs)
	want := []string{"different-date/detail.json", "match/detail.json", "secondary/document.json"}
	if !reflect.DeepEqual(evidence.refs, want) {
		t.Fatalf("refs=%v want=%v", evidence.refs, want)
	}
}
