package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func evalReleaseItem(t *testing.T, artifacts artifact.Store, body string) (releaseItem, string) {
	t.Helper()
	hash := strings.Repeat("a", 64)
	art, err := artifacts.Publish(context.Background(), "eval", hash, hash, func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "report.json"), []byte(body), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return releaseItem{Stage: "eval", State: "done", Key: "vn/20990102", Path: art.Path, ArtifactHash: art.SHA256}, sha256Hex([]byte(body))
}

func decisionSnapshot(t *testing.T, decision *stage.PreviousReport) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(stage.Snapshot{PreviousReport: decision})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

var evidenceFiles = map[string]string{"findings.json": "{\"schema\":2}\n", "identity.jsonl": "{\"schema\":1}\n", "changes.jsonl": "{\"schema\":1,\"baseline\":true}\n"}

func evidenceReleaseItem(t *testing.T, artifacts artifact.Store, rel string, files map[string]string) releaseItem {
	t.Helper()
	hash := strings.Repeat("e", 64)
	art, err := artifacts.Publish(context.Background(), stage.EvidenceStage, hash, sha256Hex([]byte(fmt.Sprint(files))), func(dir string) error {
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return releaseItem{Stage: stage.EvidenceStage, State: "done", Key: "vn/" + rel, Path: art.Path, ArtifactHash: art.SHA256}
}

func TestReleaseDecision_AddsFiveHashesFromVerifiedEvidence(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	eval, sum := evalReleaseItem(t, artifacts, `{"status":"complete"}`)
	evidence := evidenceReleaseItem(t, artifacts, "20990102", evidenceFiles)
	previous := strings.Repeat("b", 64)
	for _, tc := range []struct {
		name         string
		decision     stage.PreviousReport
		wantPrevious string
	}{
		{"report", stage.PreviousReport{SHA256: previous}, previous},
		{"none", stage.PreviousReport{SHA256: stage.NoPreviousReport}, "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			summary := releaseRun{}
			done, err := applyReleaseDecision(&summary, "vn", "20990102", decisionSnapshot(t, &tc.decision), []releaseItem{eval, evidence}, artifacts)
			if err != nil {
				t.Fatal(err)
			}
			want := releaseRun{EvalReportSHA256: sum, PreviousReportSHA256: tc.wantPrevious, FindingsSHA256: sha256Hex([]byte(evidenceFiles["findings.json"])), IdentitySHA256: sha256Hex([]byte(evidenceFiles["identity.jsonl"])), ChangesSHA256: sha256Hex([]byte(evidenceFiles["changes.jsonl"]))}
			if !done || summary != want {
				t.Fatalf("done=%v summary=%+v", done, summary)
			}
		})
	}
}

func TestReleaseDecision_DecidedRunAtTheCutoffWritesAllFive(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	cutoff := stage.LegacyCutoff("vn")
	eval, _ := evalReleaseItem(t, artifacts, `{"status":"complete"}`)
	eval.Key = "vn/" + cutoff
	decision := stage.PreviousReport{SHA256: strings.Repeat("b", 64)}
	summary := releaseRun{}
	if _, err := applyReleaseDecision(&summary, "vn", cutoff, decisionSnapshot(t, &decision), []releaseItem{eval, evidenceReleaseItem(t, artifacts, cutoff, evidenceFiles)}, artifacts); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"eval_report_sha256", "previous_report_sha256", "findings_sha256", "identity_sha256", "changes_sha256"} {
		if !strings.Contains(string(data), `"`+field+`":"`) {
			t.Errorf("decided run at the cutoff omits %s: %s", field, data)
		}
	}
	if _, err = applyReleaseDecision(&releaseRun{}, "vn", cutoff, decisionSnapshot(t, &decision), []releaseItem{eval}, artifacts); err == nil {
		t.Fatal("decided run at the cutoff without evidence wrote a row")
	}
}

func TestReleaseDecision_RefusesMissingOrUnverifiedEvidence(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	eval, _ := evalReleaseItem(t, artifacts, `{"status":"complete"}`)
	decision := stage.PreviousReport{SHA256: strings.Repeat("b", 64)}
	partial := map[string]string{"findings.json": evidenceFiles["findings.json"]}
	tampered := evidenceReleaseItem(t, artifacts, "20990102", evidenceFiles)
	tampered.ArtifactHash = strings.Repeat("0", 64)
	edited := evidenceReleaseItem(t, artifacts, "20990102", map[string]string{"findings.json": "{}", "identity.jsonl": "{}", "changes.jsonl": "{}"})
	if err := os.WriteFile(filepath.Join(artifacts.Abs(edited.Path), "identity.jsonl"), []byte("{\"x\":1}"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, items := range map[string][]releaseItem{
		"evidence missing": {eval},
		"evidence pending": {eval, {Stage: stage.EvidenceStage, State: "pending", Key: "vn/20990102"}},
		"file missing":     {eval, evidenceReleaseItem(t, artifacts, "20990102", partial)},
		"other manifest":   {eval, tampered},
		"edited file":      {eval, edited},
	} {
		t.Run(name, func(t *testing.T) {
			summary := releaseRun{}
			if _, err := applyReleaseDecision(&summary, "vn", "20990102", decisionSnapshot(t, &decision), items, artifacts); err == nil || !strings.Contains(err.Error(), "evidence") {
				t.Fatalf("err = %v summary=%+v", err, summary)
			}
		})
	}
}

func TestReleaseDecision_OlderRunWritesNeitherField(t *testing.T) {
	summary := releaseRun{ID: 7}
	done, err := applyReleaseDecision(&summary, "vn", "20261118_1", decisionSnapshot(t, nil), nil, artifact.Store{Root: t.TempDir()})
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "report_sha256") {
		t.Fatalf("older run row = %s", data)
	}
}

func TestReleaseDecision_RefusesMissingDecisionAboveCutoff(t *testing.T) {
	summary := releaseRun{}
	if _, err := applyReleaseDecision(&summary, "vn", "20261118_3", decisionSnapshot(t, nil), nil, artifact.Store{Root: t.TempDir()}); !errors.Is(err, stage.ErrNoDecision) {
		t.Fatalf("err = %v", err)
	}
}

func TestReleaseDecision_VerificationErrorNamesCause(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	eval, _ := evalReleaseItem(t, artifacts, `{"status":"complete"}`)
	decision := stage.PreviousReport{SHA256: stage.NoPreviousReport}
	other := eval
	other.ArtifactHash = strings.Repeat("0", 64)
	missing := eval
	missing.Path = "eval/absent"
	for name, tc := range map[string]struct {
		item releaseItem
		want string
	}{
		"manifest differs": {other, other.ArtifactHash},
		"files unreadable": {missing, "inspect artifact"},
	} {
		t.Run(name, func(t *testing.T) {
			summary := releaseRun{}
			_, err := applyReleaseDecision(&summary, "vn", "20990102", decisionSnapshot(t, &decision), []releaseItem{tc.item}, artifacts)
			if err == nil || !strings.Contains(err.Error(), "evaluation artifact failed verification") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestReleaseDecision_RefusesUnverifiableReport(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	eval, _ := evalReleaseItem(t, artifacts, `{"status":"complete"}`)
	decision := stage.PreviousReport{SHA256: stage.NoPreviousReport}
	tampered := eval
	tampered.ArtifactHash = strings.Repeat("0", 64)
	badDecision := stage.PreviousReport{SHA256: "NOT-HEX"}
	for name, tc := range map[string]struct {
		decision stage.PreviousReport
		items    []releaseItem
	}{
		"no eval item":   {decision, nil},
		"other manifest": {decision, []releaseItem{tampered}},
		"bad decision":   {badDecision, []releaseItem{eval}},
	} {
		t.Run(name, func(t *testing.T) {
			summary := releaseRun{}
			if _, err := applyReleaseDecision(&summary, "vn", "20990102", decisionSnapshot(t, &tc.decision), tc.items, artifacts); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
