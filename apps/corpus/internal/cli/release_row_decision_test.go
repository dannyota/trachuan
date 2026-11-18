package cli

import (
	"context"
	"encoding/json"
	"errors"
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

func TestReleaseDecision_AddsHashesAndNeedsEvidence(t *testing.T) {
	artifacts := artifact.Store{Root: t.TempDir()}
	eval, sum := evalReleaseItem(t, artifacts, `{"status":"complete"}`)
	previous := strings.Repeat("b", 64)
	evidence := releaseItem{Stage: stage.EvidenceStage, State: "done", Key: "vn/20990102"}
	for _, tc := range []struct {
		name         string
		decision     stage.PreviousReport
		items        []releaseItem
		wantPrevious string
		wantEvidence bool
	}{
		{"report with evidence", stage.PreviousReport{SHA256: previous}, []releaseItem{eval, evidence}, previous, true},
		{"none with evidence", stage.PreviousReport{SHA256: stage.NoPreviousReport}, []releaseItem{eval, evidence}, "none", true},
		{"evidence missing", stage.PreviousReport{SHA256: previous}, []releaseItem{eval}, previous, false},
		{"evidence pending", stage.PreviousReport{SHA256: previous}, []releaseItem{eval, {Stage: stage.EvidenceStage, State: "pending", Key: "vn/20990102"}}, previous, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			summary := releaseRun{}
			done, err := applyReleaseDecision(&summary, "vn", "20990102", decisionSnapshot(t, &tc.decision), tc.items, artifacts)
			if err != nil {
				t.Fatal(err)
			}
			if done != tc.wantEvidence || summary.EvalReportSHA256 != sum || summary.PreviousReportSHA256 != tc.wantPrevious {
				t.Fatalf("done=%v summary=%+v", done, summary)
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
