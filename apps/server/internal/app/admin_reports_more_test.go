package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
	"gopkg.in/yaml.v3"
)

type reportsFixture struct {
	triageFixture
	decided   int
	decisions []store.DecideReportsParams
	batches   map[string]store.BatchSnapshotRow
}

func (s *reportsFixture) TriageReports(context.Context, store.TriageReportsParams) ([]store.IssueReport, error) {
	return []store.IssueReport{{ID: 12, AccountID: 987654, Release: "20261111", State: "open", Suggestion: json.RawMessage(`{}`)}}, nil
}
func (s *reportsFixture) DecideReports(_ context.Context, p store.DecideReportsParams) (int64, error) {
	s.decided++
	s.decisions = append(s.decisions, p)
	if p.DecidedBy == nil || *p.DecidedBy != "operator" {
		return 0, nil
	}
	return 1, nil
}
func (s *reportsFixture) decideReportIDs(ctx context.Context, p store.DecideReportsParams) (int64, error) {
	for _, id := range p.Ids {
		if id != 12 && id != 13 {
			return 0, errors.New("unknown report id")
		}
	}
	return s.DecideReports(ctx, p)
}
func (s *reportsFixture) createReportsBatch(_ context.Context, id string) error {
	if s.batches == nil {
		s.batches = map[string]store.BatchSnapshotRow{}
	}
	s.batches[id] = store.BatchSnapshotRow{Reports: json.RawMessage(`[{"account_id":987654,"email":"identity@example.invalid","report_id":12,"created_at":"2026-11-11T00:00:00Z","document_key":"vn:1","anchor":"","document_number":"1/2024/QH15","release":"20261111","kind":"wrong_validity","target":"","suggestion":{"status_class":"in_force"}}]`)}
	return nil
}
func (s *reportsFixture) reportsSnapshot(_ context.Context, id string) (store.BatchSnapshotRow, error) {
	return s.batches[id], nil
}
func runFixture(s *reportsFixture, args ...string) (string, error) {
	var b bytes.Buffer
	e := runAdmin(context.Background(), append([]string{"reports"}, args...), s, &b)
	return b.String(), e
}
func TestTriage_AmbiguousSelectorRefused(t *testing.T) {
	s := &reportsFixture{}
	err := decideReports(context.Background(), s, "accept", []string{"aaaaaaaaaaaa", "--by", "operator"}, &bytes.Buffer{}, func(store.TriageGroupsRow) string { return strings.Repeat("a", 64) })
	if err == nil || s.decided != 0 {
		t.Fatal("ambiguous selector changed reports")
	}

}
func TestTriage_NoIdentityWithoutFlag(t *testing.T) {
	s := &reportsFixture{}
	gs, _ := s.TriageGroups(context.Background(), store.TriageGroupsParams{})
	id := groupID(gs[0])
	for _, tc := range []struct {
		args []string
		want bool
	}{{[]string{"show", id}, false}, {[]string{"show", id, "--accounts"}, true}} {
		out, e := runFixture(s, tc.args...)
		if e != nil || strings.Contains(out, "987654") != tc.want {
			t.Fatalf("show: %q %v", out, e)
		}
	}
}
func TestDecide_GroupAndIDs(t *testing.T) {
	s := &reportsFixture{}
	gs, _ := s.TriageGroups(context.Background(), store.TriageGroupsParams{})
	for _, args := range [][]string{{"accept", groupID(gs[0])[:12], "--by", "operator"}, {"reject", "--ids", "12,13", "--by", "operator"}} {
		if _, e := runFixture(s, args...); e != nil {
			t.Fatal(e)
		}
	}
	if s.decided != 2 {
		t.Fatal("decisions missing")
	}
	first, second := s.decisions[0], s.decisions[1]
	if first.Decision != "accepted" || first.GroupTarget == nil || *first.GroupTarget != "1/2024/ND-CP" || len(first.Ids) != 0 {
		t.Fatalf("group selection: %+v", first)
	}
	if second.Decision != "rejected" || second.GroupDocumentKey != nil || len(second.Ids) != 2 || second.Ids[0] != 12 || second.Ids[1] != 13 {
		t.Fatalf("id selection: %+v", second)
	}
}
func exportFixture(t *testing.T) (*reportsFixture, string, string) {
	t.Helper()
	s := &reportsFixture{}
	dir := t.TempDir()
	out, e := runFixture(s, "export", "--out", dir)
	if e != nil {
		t.Fatal(e)
	}
	id := strings.TrimSpace(out)
	return s, dir, id
}
func TestExport_DraftFormats(t *testing.T) {
	_, dir, id := exportFixture(t)
	data, e := os.ReadFile(filepath.Join(dir, id, "corrections-draft.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	var entries []struct {
		Key    string `yaml:"key"`
		Number string `yaml:"number"`
		Set    struct {
			StatusClass *string `yaml:"status_class"`
			EffFrom     *string `yaml:"eff_from"`
			EffTo       *string `yaml:"eff_to"`
		} `yaml:"set"`
		Eligible *bool  `yaml:"eligible"`
		Hold     bool   `yaml:"hold"`
		Reason   string `yaml:"reason"`
		Evidence struct {
			Key    string `yaml:"key"`
			Anchor string `yaml:"anchor"`
			URL    string `yaml:"url"`
			Quote  string `yaml:"quote"`
		} `yaml:"evidence"`
		Reviewed string `yaml:"reviewed"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if e = decoder.Decode(&entries); e != nil || len(entries) != 1 || entries[0].Key != "vn:1" || entries[0].Number != "1/2024/QH15" || entries[0].Set.StatusClass == nil || *entries[0].Set.StatusClass != "in_force" || entries[0].Evidence.Quote != "TODO" || entries[0].Reason != "TODO" || entries[0].Reviewed != "" {
		t.Fatalf("draft: %s %v", data, e)
	}
}
func TestExport_NoAccountData(t *testing.T) {
	_, dir, id := exportFixture(t)
	files, e := os.ReadDir(filepath.Join(dir, id))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range files {
		b, e := os.ReadFile(filepath.Join(dir, id, f.Name()))
		if e != nil || bytes.Contains(b, []byte("account")) || bytes.Contains(b, []byte("987654")) || bytes.Contains(b, []byte("identity@example.invalid")) {
			t.Fatalf("identity in %s", f.Name())
		}
	}
}
func TestExport_BatchRecovery(t *testing.T) {
	s, dir, id := exportFixture(t)
	if _, e := runFixture(s, "export", "--out", dir, "--batch", id); e == nil {
		t.Fatal("existing directory accepted")
	}
	other := t.TempDir()
	if _, e := runFixture(s, "export", "--out", other, "--batch", id); e != nil {
		t.Fatal(e)
	}
	s.batches[id] = store.BatchSnapshotRow{Expired: true}
	if _, e := runFixture(s, "export", "--out", t.TempDir(), "--batch", id); e == nil || e.Error() != "batch expired" {
		t.Fatalf("expired: %v", e)
	}
	if _, e := runFixture(s, "export", "--out", t.TempDir(), "--batch", "missing"); e == nil || e.Error() != "batch unavailable" {
		t.Fatalf("unavailable: %v", e)
	}
}
func TestExport_OnlyUnbatchedReports(t *testing.T) {
	s, _, id := exportFixture(t)
	if id == "" || len(s.batches) != 1 {
		t.Fatal("batch not created")
	}
	_, e := runFixture(s, "export", "--out", t.TempDir(), "--batch", id)
	if e != nil || len(s.batches) != 1 {
		t.Fatal("recovery created batch")
	}
}

func TestExport_ValidityEligibility(t *testing.T) {
	dir := t.TempDir()
	row := func(id int64, key, anchor, kind string, suggestion map[string]string) exportReport {
		return exportReport{ReportID: id, DocumentKey: key, Anchor: anchor, DocumentNumber: "1/2024/QH15", Kind: kind, Suggestion: suggestion}
	}
	rows := []exportReport{
		row(1, "agreed", "", "wrong_validity", map[string]string{"status_class": "in_force"}),
		row(2, "agreed", "", "wrong_validity", map[string]string{"status_class": "in_force"}),
		row(3, "conflict", "", "wrong_validity", map[string]string{"status_class": "in_force"}),
		row(4, "conflict", "", "wrong_validity", map[string]string{"status_class": "expired"}),
		row(5, "empty", "", "wrong_validity", map[string]string{}),
		row(6, "provision", "dieu-1", "wrong_validity", map[string]string{"status_class": "in_force"}),
		row(7, "text", "", "wrong_text", map[string]string{}),
		row(8, "citation", "", "broken_citation", map[string]string{}),
	}
	if err := writeReportExport(dir, "batch", rows); err != nil {
		t.Fatal(err)
	}
	for name, expect := range map[string][]string{"corrections-draft.yaml": {"agreed", "TODO"}, "validity-review.jsonl": {"conflict", "empty", "provision"}, "text-issues.jsonl": {"text"}, "citation-review.jsonl": {"citation"}} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range expect {
			if !strings.Contains(string(b), value) {
				t.Errorf("%s missing %s", name, value)
			}
		}
		if name == "corrections-draft.yaml" && (strings.Contains(string(b), "conflict") || strings.Contains(string(b), "empty") || strings.Contains(string(b), "provision")) {
			t.Fatal("ineligible correction drafted")
		}
	}
}

func TestDecide_UnknownIDRefused(t *testing.T) {
	s := &reportsFixture{}
	if _, err := runFixture(s, "accept", "--ids", "12,99", "--by", "operator"); err == nil || s.decided != 0 {
		t.Fatal("unknown report id changed reports")
	}
}

func TestTriage_ShowIncludesGroup(t *testing.T) {
	s := &reportsFixture{}
	gs, _ := s.TriageGroups(context.Background(), store.TriageGroupsParams{})
	out, err := runFixture(s, "show", groupID(gs[0])[:12])
	if err != nil || !strings.Contains(out, "1/2024/ND-CP") || !strings.Contains(out, "broken_citation") {
		t.Fatalf("show group missing: %q %v", out, err)
	}
}
