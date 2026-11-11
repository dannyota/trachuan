package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestBuild_DevelopmentMappingsValidated(t *testing.T) {
	suite := evaluation.Suite{Legacy: []evaluation.Case{{ID: "old", ExpectedCitations: []evaluation.Expectation{{DocNumber: "1/2026/TT"}}}}, Development: []evaluation.Case{{ID: "development", ExpectedCitations: []evaluation.Expectation{{DocNumber: "2/2026/TT"}}}}, Hashes: evaluation.FixtureHashes{Legacy: "legacy"}}
	mapping := evaluation.IdentityMap{Version: 2, LegacySHA256: "legacy"}
	for _, c := range append(append([]evaluation.Case{}, suite.Legacy...), suite.Development...) {
		number := c.ExpectedCitations[0].DocNumber
		mapping.Entries = append(mapping.Entries, evaluation.MapEntry{CaseID: c.ID, State: "resolved", ExpectedNumber: number, Reason: "reviewed", ReviewedAt: "2026-11-11", Evidence: []evaluation.Evidence{{DocumentKey: c.ID, DocumentNumber: number, FetchSHA256: "hash", QuotedSpan: "reviewed"}}, Alternatives: []evaluation.Alternative{{Country: "vn", DocumentKey: c.ID, DocumentNumber: number, Kind: "document"}}})
	}
	data, err := json.Marshal(mapping)
	if err != nil {
		t.Fatal(err)
	}
	suite.MapBytes = data
	suite.Hashes.IdentityMap = fmt.Sprintf("%x", sha256.Sum256(data))
	s := New(Config{Suite: suite})
	inv := evaluation.IdentityInventory{Documents: map[string]evaluation.IdentityDocument{"old": {Number: "1/2026/TT"}, "development": {Number: "2/2026/TT"}}}
	report, err := s.validateMappings(inv)
	if err != nil {
		t.Fatalf("valid development mapping rejected: %v", err)
	}
	if len(report) != 0 {
		t.Fatalf("valid mappings have issues: %+v", report)
	}
	delete(inv.Documents, "development")
	report, err = s.validateMappings(inv)
	if err != nil {
		t.Fatal(err)
	}
	if len(report["development"]) == 0 || len(report["old"]) != 0 {
		t.Fatalf("development identity not validated separately: %+v", report)
	}
}

func TestBuild_DevelopmentHashInRecipe(t *testing.T) {
	snapshot := stage.Snapshot{SourceList: []byte(`{"country":"vn","status_codes":{}}`)}
	s := New(Config{Suite: evaluation.Suite{Hashes: evaluation.FixtureHashes{Development: "first"}}})
	first := s.Recipe(snapshot)
	s.cfg.Suite.Hashes.Development = "second"
	second := s.Recipe(snapshot)
	if first == "" || second == "" || first == second {
		t.Fatal("development fixture omitted from build recipe")
	}
}

func TestBuild_DevelopmentHashInInput(t *testing.T) {
	for _, tc := range []struct {
		version             int
		recordedCorrections string
	}{{0, ""}, {2, hashBytes([]byte("[]"))}} {
		version := tc.version
		inv := Inventory{Version: version, Country: "vn"}
		s := New(Config{Suite: evaluation.Suite{Hashes: evaluation.FixtureHashes{IdentityMap: "map", Development: "first"}}})
		correction := correctionInput{hash: hashBytes([]byte("[]"))}
		first := s.inputHash("vn", "20990101", inv, correction)
		restored := recordInputHash("vn", "20990101", inv, "map", hashJSON(inv.Provenance), tc.recordedCorrections, "first")
		if restored != first {
			t.Fatalf("inventory version %d restore hash differs from planned input", version)
		}
		if restored == recordInputHash("vn", "20990101", inv, "map", hashJSON(inv.Provenance), tc.recordedCorrections, "changed") {
			t.Fatal("restore hash omitted development fixture")
		}
		s.cfg.Suite.Hashes.Development = "second"
		second := s.inputHash("vn", "20990101", inv, correction)
		if first == second {
			t.Fatalf("inventory version %d omitted development fixture", version)
		}
	}
}
