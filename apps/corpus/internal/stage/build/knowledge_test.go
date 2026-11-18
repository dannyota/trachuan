package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/knowledge"
)

const knowledgeDecision = `- id: "decision-mau"
  kind: "review_decision"
  target: { finding: "sha256:1111111111111111111111111111111111111111111111111111111111111111" }
  claim: { reason_code: "expected", reason: "Why it needs no change." }
  depends: { fingerprint: "sha256:2222222222222222222222222222222222222222222222222222222222222222", semantics: 1, first_rel: "20991118_1" }
  reviewed: { date: "2099-11-18", by: "primary", method: "human" }
`

func knowledgeInputFile(t *testing.T, content string) KnowledgeInput {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vn-knowledge.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	input, err := ReadKnowledge("vn", path)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestReadKnowledge_RequiresFileAndRefusesInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vn-knowledge.yaml")
	if _, err := ReadKnowledge("vn", path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if err := os.WriteFile(path, []byte("- id: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKnowledge("vn", path); err == nil {
		t.Fatal("invalid file accepted")
	}
}

func TestReadKnowledge_HoldsBytesReadOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vn-knowledge.yaml")
	if err := os.WriteFile(path, []byte(knowledgeDecision), 0o600); err != nil {
		t.Fatal(err)
	}
	input, err := ReadKnowledge("vn", path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if string(input.data) != knowledgeDecision || input.hash != hashBytes([]byte(knowledgeDecision)) {
		t.Fatalf("input = %d bytes, hash %s", len(input.data), input.hash)
	}
}

func TestBuildInput_KnowledgeOnlyWithEntries(t *testing.T) {
	inv := Inventory{Country: "vn"}
	today := New(Config{}).inputHash("vn", "20990101", inv, correctionInput{})
	for _, content := range []string{"", "# none yet\n", "[]\n"} {
		cfg := Config{Knowledge: knowledgeInputFile(t, content)}
		if got := New(cfg).inputHash("vn", "20990101", inv, correctionInput{}); got != today {
			t.Fatalf("no entries (%q) changed the input hash", content)
		}
	}
	one := New(Config{Knowledge: knowledgeInputFile(t, knowledgeDecision)}).inputHash("vn", "20990101", inv, correctionInput{})
	if one == today || one == "" {
		t.Fatalf("entry left the hash at %s", one)
	}
	if want := withKnowledge(today, hashBytes([]byte(knowledgeDecision))); one != want {
		t.Fatalf("hash = %s, want %s", one, want)
	}
	two := New(Config{Knowledge: knowledgeInputFile(t, knowledgeDecision+"# comment\n")}).inputHash("vn", "20990101", inv, correctionInput{})
	if two == one {
		t.Fatal("raw bytes do not reach the input hash")
	}
}

func TestBuildInput_RecordMatchesKnowledgeHash(t *testing.T) {
	inv := Inventory{Country: "vn"}
	cfg := Config{Knowledge: knowledgeInputFile(t, knowledgeDecision)}
	_, exclusions, err := cfg.HeadingExclusions.pinned()
	if err != nil {
		t.Fatal(err)
	}
	got := New(cfg).inputHash("vn", "20990101", inv, correctionInput{})
	recorded := recordInputHash("vn", "20990101", inv, "", provenanceInputHash(inv), "", "", "", exclusions)
	if want := withKnowledge(recorded, cfg.Knowledge.hash); got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if withKnowledge(recorded, "") != recorded {
		t.Fatal("empty knowledge changed the record hash")
	}
}

func TestKnowledgeArtifact_NoEntriesWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if err := writeKnowledgeArtifact(dir, knowledgeInputFile(t, "[]\n"), knowledge.Report{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries = %v, %v", entries, err)
	}
	hash, err := knowledgeArtifactHash(dir)
	if err != nil || hash != "" {
		t.Fatalf("hash = %q, %v", hash, err)
	}
}

func TestKnowledgeArtifact_EntriesWriteBytesAndReport(t *testing.T) {
	dir := t.TempDir()
	input := knowledgeInputFile(t, knowledgeDecision)
	report := knowledge.Report{Records: []knowledge.Record{{ID: "effect-a", Kind: knowledge.KindEffect, TargetKey: "vn:vbpl:1", Outcome: knowledge.OutcomeStale, Reason: "text"}}}
	if err := writeKnowledgeArtifact(dir, input, report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "knowledge.yaml"))
	if err != nil || string(data) != knowledgeDecision {
		t.Fatalf("knowledge.yaml = %q, %v", data, err)
	}
	var records []knowledge.Record
	raw, err := os.ReadFile(filepath.Join(dir, "knowledge-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &records); err != nil || len(records) != 1 || records[0] != report.Records[0] {
		t.Fatalf("records = %+v, %v", records, err)
	}
	hash, err := knowledgeArtifactHash(dir)
	if err != nil || hash != input.hash {
		t.Fatalf("hash = %q, %v", hash, err)
	}
}

func TestKnowledgeArtifact_DecisionsOnlyReportIsEmptyList(t *testing.T) {
	dir := t.TempDir()
	if err := writeKnowledgeArtifact(dir, knowledgeInputFile(t, knowledgeDecision), knowledge.Report{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "knowledge-report.json"))
	if err != nil || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("report = %q, %v", raw, err)
	}
}

func TestKnowledgeArtifact_RefusesHalf(t *testing.T) {
	for _, name := range []string{"knowledge.yaml", "knowledge-report.json"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := knowledgeArtifactHash(dir); err == nil {
			t.Fatalf("%s alone accepted", name)
		}
	}
}

func TestKnowledgeReader_BaselineComesFromBuildGaps(t *testing.T) {
	reader := knowledgeReader{gaps: []InventoryGap{
		{Stage: "build", ItemKey: "vn:vbpl:1", Kind: "baseline_served"},
		{Stage: "fetch", ItemKey: "vn:vbpl:2", Kind: "baseline_served"},
		{Stage: "build", ItemKey: "vn:vbpl:3", Kind: "source_unavailable"},
		{Stage: "build", ItemKey: "vn:vbpl:4", Kind: "baseline_served"},
	}}
	got, err := reader.BaselineServed(context.Background(), []string{"vn:vbpl:1", "vn:vbpl:2", "vn:vbpl:3"})
	if err != nil || len(got) != 1 || got[0] != "vn:vbpl:1" {
		t.Fatalf("baseline = %v, %v", got, err)
	}
}

func TestValidityRelations_RulesSkipReviewedPartialReviewReads(t *testing.T) {
	relations := []validityRelation{{source: "a", target: "t", kind: "repeals", method: knowledge.MethodReviewed, id: 1}, {source: "b", target: "t", kind: "repeals", method: "diagram", id: 2}}
	rules := ruleRelations(relations)
	if len(rules) != 1 || rules[0].id != 2 {
		t.Fatalf("rules = %+v", rules)
	}
	docs := map[string]validityDocument{"a": {key: "a", number: "1"}, "t": {key: "t", number: "9", sourceClass: "partial"}}
	partial := partialDocuments(docs, relations[:1])
	if len(partial) != 1 || len(partial[0].Acting) != 1 || partial[0].Acting[0].Key != "a" {
		t.Fatalf("partial = %+v", partial)
	}
}

func TestSummaryGap_KnowledgeStaleIsNotStructural(t *testing.T) {
	structural, err := structuralSummaryGap("knowledge_stale")
	if err != nil || structural {
		t.Fatalf("structural=%v err=%v", structural, err)
	}
}

func TestDeriveValidity_ReviewedRowsNeverReachRules(t *testing.T) {
	ruleCTarget := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	ruleCActor := testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")
	ruleCActor.article = testArticle("1/2/2021", "")
	ruleATarget := testDoc("a", "1/2021/NĐ-CP", "2021-01-01", "2098-01-01")
	ruleATarget.sourceClass = "not_yet"
	ruleATarget.article = testArticle("1/2/2021", "")
	ruleAActor := testDoc("b", "2/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	for name, tc := range map[string]struct {
		target, actor validityDocument
		kind          string
	}{
		"rule C conflict":          {ruleCTarget, ruleCActor, "replaces"},
		"rule A incoming relation": {ruleATarget, ruleAActor, "suspends"},
	} {
		t.Run(name, func(t *testing.T) {
			docs := map[string]validityDocument{"a": tc.target, "b": tc.actor}
			derive := func(relations []validityRelation) validityResult {
				t.Helper()
				result, err := deriveValidity(docs, relations, nil, "2099-01-01")
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			row := func(method string) []validityRelation {
				return []validityRelation{{source: "b", target: "a", kind: tc.kind, method: method, id: 1}}
			}
			plain, reviewed, diagram := derive(nil), derive(row(knowledge.MethodReviewed)), derive(row("diagram"))
			if reflect.DeepEqual(plain.documents, diagram.documents) {
				t.Fatal("fixture: a diagram row changes nothing, so the test proves nothing")
			}
			if !reflect.DeepEqual(plain.documents, reviewed.documents) {
				t.Fatalf("reviewed row changed documents:\n%+v\n%+v", plain.documents, reviewed.documents)
			}
			plainReview, err := encodeValidityReview(plain.reviews)
			if err != nil {
				t.Fatal(err)
			}
			reviewedReview, err := encodeValidityReview(reviewed.reviews)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(plainReview, reviewedReview) {
				t.Fatalf("reviewed row changed reviews:\n%s\n%s", plainReview, reviewedReview)
			}
		})
	}
}

func TestDeriveValidity_PartialReviewReadsReviewedRows(t *testing.T) {
	target := testDoc("a", "1/2020/NĐ-CP", "2020-01-01", "2020-01-01")
	target.sourceClass = "partial"
	docs := map[string]validityDocument{"a": target, "b": testDoc("b", "2/2021/NĐ-CP", "2021-01-01", "2021-02-01")}
	result, err := deriveValidity(docs, []validityRelation{{source: "b", target: "a", kind: "repeals", method: knowledge.MethodReviewed, id: 1}}, nil, "2099-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.partialDocuments) != 1 || len(result.partialDocuments[0].Acting) != 1 {
		t.Fatalf("partial = %+v", result.partialDocuments)
	}
}
