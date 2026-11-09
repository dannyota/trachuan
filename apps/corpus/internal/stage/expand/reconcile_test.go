package expand

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func testSeed(id string) membership.Entry {
	return membership.Entry{DocumentKey: "vn:vbpl:" + id, Source: "vbpl", SourceID: id, Class: "primary", Disposition: "active"}
}
func testContext(id, seed string) membership.Entry {
	e := testSeed(id)
	e.Class = "relation_context"
	e.Supports = []membership.SupportTuple{{RuleKind: "relation", RuleValue: "vn:vbpl:" + seed + "|A", Evidence: membership.EvidenceRef{ResponseSHA256: "response", ArtifactHash: "artifact", Path: "detail.json"}}}
	return e
}
func TestExpand_FailedObservationRetainsTargets(t *testing.T) {
	members := reconcile([]membership.Entry{testSeed("1")}, []membership.Entry{testContext("2", "1")}, map[string]seedObservation{"vn:vbpl:1": {}})
	m := members["vn:vbpl:2"]
	if m == nil || m.entry.Disposition != "active" || len(m.records) != 1 || m.records[0].State != "retained" {
		t.Fatalf("member = %#v", m)
	}
}
func TestExpand_SourceFailureKeepsKnownTargets(t *testing.T) {
	prior := testContext("3", "1")
	prior.Supports = append(prior.Supports, testContext("3", "2").Supports...)
	members := reconcile([]membership.Entry{testSeed("1"), testSeed("2")}, []membership.Entry{prior}, map[string]seedObservation{"vn:vbpl:1": {complete: true}, "vn:vbpl:2": {}})
	m := members["vn:vbpl:3"]
	if m == nil || m.entry.Disposition != "active" || len(m.records) != 2 || m.records[0].State != "removed" || m.records[1].State != "retained" {
		t.Fatalf("member = %#v", m)
	}
}
func TestExpand_RemovalNeedsCompleteCurrentSupport(t *testing.T) {
	members := reconcile([]membership.Entry{testSeed("1")}, []membership.Entry{testContext("2", "1")}, map[string]seedObservation{"vn:vbpl:1": {complete: true}})
	m := members["vn:vbpl:2"]
	if m == nil || m.entry.Disposition != "withdrawn" || m.records[0].State != "removed" || m.records[0].Stage != "expand" {
		t.Fatalf("member = %#v", m)
	}
}
func TestExpand_DepartedSeedDropsSupport(t *testing.T) {
	members := reconcile(nil, []membership.Entry{testContext("2", "1")}, nil)
	m := members["vn:vbpl:2"]
	if m == nil || m.entry.Disposition != "scope_removed" || m.records[0].State != "removed" || m.records[0].Stage != "discover" || m.records[0].ItemKey != "vbpl:central" {
		t.Fatalf("member = %#v", m)
	}
}
func TestExpand_ReobservedIdenticalSupportStaysCurrent(t *testing.T) {
	hash := "artifact"
	prior := testContext("2", "1")
	obs := seedObservation{complete: true, artifactHash: &hash, observation: Observation{Targets: []Target{{TargetKey: "vn:vbpl:2", RawCode: "A", Response: "detail.json", ResponseSHA256: "response"}}}}
	members := reconcile([]membership.Entry{testSeed("1")}, []membership.Entry{prior}, map[string]seedObservation{"vn:vbpl:1": obs})
	records := members["vn:vbpl:2"].records
	if len(records) != 1 || records[0].State != "current" {
		t.Fatalf("records = %#v", records)
	}
}

func TestExpand_TargetInheritsCountryKey(t *testing.T) {
	hash := "artifact"
	seed := membership.Entry{DocumentKey: "kh:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}
	obs := seedObservation{complete: true, artifactHash: &hash, observation: Observation{Targets: []Target{{TargetKey: "kh:vbpl:2", RawCode: "A", Response: "detail.json", ResponseSHA256: "response"}}}}
	got := reconcile([]membership.Entry{seed}, nil, map[string]seedObservation{seed.DocumentKey: obs})["kh:vbpl:2"]
	if got == nil || got.entry.SourceID != "2" {
		t.Fatalf("entry = %#v", got)
	}
}

func TestExpand_PriorContextRemovalOverridesRemovedSeedAudit(t *testing.T) {
	seed := testSeed("1")
	prior := testContext("2", "1")
	removed := testSeed("2")
	removed.Disposition = "scope_removed"
	members := reconcile([]membership.Entry{seed, removed}, []membership.Entry{prior}, map[string]seedObservation{"vn:vbpl:1": {complete: true}})
	if got := members["vn:vbpl:2"].entry.Disposition; got != "withdrawn" {
		t.Fatalf("disposition = %q", got)
	}
}

func TestExpand_NewEvidenceSupersedesPriorRule(t *testing.T) {
	old := testContext("2", "1")
	hash := "new artifact"
	obs := seedObservation{complete: true, artifactHash: &hash, observation: Observation{Targets: []Target{{TargetKey: "vn:vbpl:2", RawCode: "A", Response: "detail.json", ResponseSHA256: "new response"}}}}
	for i := 0; i < 3; i++ {
		m := reconcile([]membership.Entry{testSeed("1")}, []membership.Entry{old}, map[string]seedObservation{"vn:vbpl:1": obs})["vn:vbpl:2"]
		if len(m.entry.Supports) != 1 || len(m.records) != 1 || m.records[0].State != "current" {
			t.Fatalf("run %d: %#v", i, m)
		}
		old = m.entry
	}
}

func TestExpand_RemovedRulePersistsWithoutDuplication(t *testing.T) {
	seeds := []membership.Entry{testSeed("1")}
	prior := testContext("2", "1")
	obs := map[string]seedObservation{"vn:vbpl:1": {complete: true}}
	for i := 0; i < 2; i++ {
		m := reconcile(seeds, []membership.Entry{prior}, obs)["vn:vbpl:2"]
		if m == nil || m.entry.Disposition != "withdrawn" || len(m.records) != 1 || m.records[0].State != "removed" || len(m.entry.Supports) != 1 {
			t.Fatalf("run %d: %#v", i, m)
		}
		prior = m.entry
	}
}

func TestExpand_RemovedTargetDoesNotReturnOnIncompleteObservation(t *testing.T) {
	prior := testContext("2", "1")
	prior.Disposition = "withdrawn"
	m := reconcile([]membership.Entry{testSeed("1")}, []membership.Entry{prior}, map[string]seedObservation{"vn:vbpl:1": {}})["vn:vbpl:2"]
	if m == nil || m.entry.Disposition != "withdrawn" || len(m.records) != 1 || m.records[0].State != "removed" {
		t.Fatalf("member=%#v", m)
	}
}
