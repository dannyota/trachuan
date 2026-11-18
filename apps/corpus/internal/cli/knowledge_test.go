package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/knowledge"
	"danny.vn/trachuan/internal/corpusdb"
)

const cliKnowledgeDecision = `- id: "decision-mau"
  kind: "review_decision"
  target: { finding: "sha256:1111111111111111111111111111111111111111111111111111111111111111" }
  claim: { reason_code: "expected", reason: "Why it needs no change." }
  depends: { fingerprint: "sha256:2222222222222222222222222222222222222222222222222222222222222222", semantics: 1, first_rel: "20991118_1" }
  reviewed: { date: "2099-11-18", by: "primary", method: "human" }
`

func execKnowledge(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errout bytes.Buffer
	code := Run(context.Background(), append([]string{"knowledge"}, args...), &out, &errout)
	return code, out.String(), errout.String()
}

func knowledgeDir(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sources"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sources", "vn-knowledge.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

func TestKnowledgeCheck_Structural(t *testing.T) {
	knowledgeDir(t, cliKnowledgeDecision)
	code, out, errout := execKnowledge(t, "check", "vn")
	if code != 0 || errout != "" || !strings.Contains(out, "validated 1 entries") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
}

func TestKnowledgeCheck_ReportsEveryProblem(t *testing.T) {
	knowledgeDir(t, "- { id: x }\n- { id: y }\n")
	code, _, errout := execKnowledge(t, "check", "vn")
	if code != 1 || strings.Count(strings.TrimSpace(errout), "\n") < 1 {
		t.Fatalf("code=%d err=%q", code, errout)
	}
}

func TestKnowledgeCheck_MissingFileFails(t *testing.T) {
	t.Chdir(t.TempDir())
	if code, _, errout := execKnowledge(t, "check", "vn"); code != 1 || !strings.Contains(errout, "vn-knowledge.yaml") {
		t.Fatalf("code=%d err=%q", code, errout)
	}
}

func TestKnowledgeCheck_ReleaseNeedsBuilderURL(t *testing.T) {
	knowledgeDir(t, "[]\n")
	t.Setenv("CORPUS_BUILDER_DATABASE_URL", "")
	if code, _, errout := execKnowledge(t, "check", "vn", "20990101"); code != 1 || !strings.Contains(errout, "CORPUS_BUILDER_DATABASE_URL") {
		t.Fatalf("code=%d err=%q", code, errout)
	}
}

func TestKnowledge_Usage(t *testing.T) {
	for _, args := range [][]string{nil, {"check"}, {"check", "vn", "20990101", "extra"}, {"depends", "vn", "20990101", "vn:vbpl:1"}, {"rebind", "vn", "20990101"}, {"check", "VN"}, {"check", "../x"}, {"check", "vn", "bad"}, {"depends", "vn", "bad", "vn:vbpl:1", "dieu-1"}} {
		if code, _, errout := execKnowledge(t, args...); code != 2 || errout == "" {
			t.Errorf("args %v: code=%d err=%q", args, code, errout)
		}
	}
}

func TestKnowledgeReport_Prints(t *testing.T) {
	var out bytes.Buffer
	report := knowledge.Report{Records: []knowledge.Record{
		{ID: "effect-a", Kind: knowledge.KindEffect, TargetKey: "vn:vbpl:1", Outcome: knowledge.OutcomeApplied},
		{ID: "effect-b", Kind: knowledge.KindEffect, TargetKey: "vn:vbpl:2", Outcome: knowledge.OutcomeStale, Reason: "text"},
		{ID: "relation-c", Kind: knowledge.KindRelation, TargetKey: "vn:vbpl:3", Outcome: knowledge.OutcomeRedundant},
	}}
	if err := printKnowledgeReport(&out, report); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"effect-a\tprovision_effect\tvn:vbpl:1\tapplied\t\n", "effect-b\tprovision_effect\tvn:vbpl:2\tstale\ttext\n", "applied 1, redundant 1, stale 1\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestRun_UsageListsKnowledge(t *testing.T) {
	var out, errout bytes.Buffer
	if code := Run(context.Background(), []string{"nope"}, &out, &errout); code != 2 || !strings.Contains(errout.String(), "knowledge") {
		t.Fatalf("code=%d err=%q", code, errout.String())
	}
}

type fakeKnowledgeRelease struct {
	present bool
	err     error
	reads   int
}

func (f *fakeKnowledgeRelease) HasRelease(_ context.Context, country, rel string, schemaVersion int) (bool, error) {
	if country != "vn" || rel != "20990101" || schemaVersion != corpusdb.SchemaVersion {
		return false, errors.New("unexpected release identity")
	}
	return f.present, f.err
}

func (f *fakeKnowledgeRelease) Documents(context.Context, []string) ([]knowledge.Document, error) {
	f.reads++
	return nil, nil
}

func (f *fakeKnowledgeRelease) BaselineServed(context.Context, []string) ([]string, error) {
	f.reads++
	return nil, nil
}

func (f *fakeKnowledgeRelease) RelationRows(context.Context, []int64) ([]knowledge.RelationRow, error) {
	f.reads++
	return nil, nil
}

func (f *fakeKnowledgeRelease) Provisions(context.Context, int64) ([]knowledge.Provision, error) {
	f.reads++
	return nil, nil
}

func (f *fakeKnowledgeRelease) ProvisionText(context.Context, int64) (string, error) {
	f.reads++
	return "", nil
}

func knowledgeClaimFile() knowledge.File {
	return knowledge.File{Claims: []knowledge.Claim{{ID: "effect-a", Kind: knowledge.KindEffect, ActingKey: "vn:vbpl:2", ActingAnchor: "dieu-1", TargetKey: "vn:vbpl:1", TargetAnchor: "dieu-2", RelationKind: "repeals", Contract: knowledge.ContractVersion}}}
}

func TestKnowledgeCheckRelease_RefusesWrongDatabaseForEveryFile(t *testing.T) {
	decisions, err := knowledge.Parse("vn", []byte(cliKnowledgeDecision))
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range map[string]knowledge.File{"no entries": {}, "decisions only": decisions, "claims": knowledgeClaimFile()} {
		t.Run(name, func(t *testing.T) {
			db := &fakeKnowledgeRelease{}
			var out, errout bytes.Buffer
			if code := checkKnowledgeRelease(context.Background(), db, file, "vn", "20990101", &out, &errout); code != 1 || !strings.Contains(errout.String(), "vn/20990101") || db.reads != 0 {
				t.Fatalf("code=%d err=%q reads=%d", code, errout.String(), db.reads)
			}
		})
	}
}

func TestKnowledgeCheckRelease_ReportsReadFailure(t *testing.T) {
	db := &fakeKnowledgeRelease{err: errors.New("no release table")}
	var out, errout bytes.Buffer
	if code := checkKnowledgeRelease(context.Background(), db, knowledge.File{}, "vn", "20990101", &out, &errout); code != 1 || !strings.Contains(errout.String(), "no release table") {
		t.Fatalf("code=%d err=%q", code, errout.String())
	}
}

func TestKnowledgeCheckRelease_ChecksOnceReleaseIsProved(t *testing.T) {
	db := &fakeKnowledgeRelease{present: true}
	var out, errout bytes.Buffer
	if code := checkKnowledgeRelease(context.Background(), db, knowledgeClaimFile(), "vn", "20990101", &out, &errout); code != 0 || !strings.Contains(out.String(), "stale\tdocument_absent") || db.reads == 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errout.String())
	}
}

func TestKnowledgeDepends_RefusesWrongDatabase(t *testing.T) {
	db := &fakeKnowledgeRelease{}
	var out, errout bytes.Buffer
	if code := printKnowledgeDepends(context.Background(), db, "vn", "20990101", "vn:vbpl:1", "dieu-1", &out, &errout); code != 1 || !strings.Contains(errout.String(), "vn/20990101") || db.reads != 0 || out.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q reads=%d", code, out.String(), errout.String(), db.reads)
	}
}
