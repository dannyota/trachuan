//go:build integration

package build

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func addValidityFixture(t *testing.T, f *loadFixture, id, number, code, issue, from, body string) string {
	t.Helper()
	key := "vn:vbpl:" + id
	data := map[string]any{"id": id, "docNum": number, "docType": map[string]any{"name": "Nghị định"}, "agencyName": "Chính phủ", "issueDate": issue, "effFrom": from, "effStatus": map[string]any{"code": code}}
	art := f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": data}, "files.json": []any{}})
	if body != "" {
		nodes := []chunk.Node{{NodeID: "article", Kind: "dieu", Label: "Điều 1", Heading: "Hiệu lực thi hành", Ordinal: 1, Path: "dieu-1", Text: "Điều 1. Hiệu lực thi hành\n" + body}}
		f.addArtifact(t, "extract", key, map[string]any{"provisions.json": nodes, "report.json": []any{}}, art.SHA256)
	}
	return art.SHA256
}
func loadValidityFixture(t *testing.T, f *loadFixture, corrections []source.Correction) error {
	t.Helper()
	codes := map[string]source.StatusCode{"CCHL": {Class: "not_yet"}, "CHL": {Class: "in_force", Current: true}}
	_, err := Load(context.Background(), f.corpus, f.inventory(t), f.artifacts, f.recipes, codes, corrections, f.asOf())
	return err
}
func TestLoad_ValidityRuleACorrectsNotYet(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CCHL", "2024-01-01", "2027-01-01", "Nghị định này có hiệu lực từ ngày 1/7/2024.")
	if err := loadValidityFixture(t, f, nil); err != nil {
		t.Fatal(err)
	}
	var class, basis, from string
	if err := f.corpus.QueryRow(context.Background(), `SELECT status_class,status_basis,eff_from::text FROM document`).Scan(&class, &basis, &from); err != nil || class != "in_force" || basis != "derived_from_text" || from != "2024-07-01" {
		t.Fatalf("%s %s %s: %v", class, basis, from, err)
	}
}
func addReplacementFixture(t *testing.T, f *loadFixture, body string) {
	t.Helper()
	addValidityFixture(t, f, "1", "1/2020/NĐ-CP", "CHL", "2020-01-01", "2020-01-01", "")
	hash := addValidityFixture(t, f, "2", "2/2021/NĐ-CP", "CHL", "2021-01-01", "2021-02-01", body)
	f.addArtifact(t, "relate", "vn:vbpl:2", map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "replaces", "target_vbpl_id": "1"}}}, f.relateInput(t, "vn:vbpl:2", hash))
}
func TestLoad_ValidityRuleBSetsPredecessorEnd(t *testing.T) {
	f := fixture(t)
	addReplacementFixture(t, f, "Nghị định này có hiệu lực từ ngày 1/2/2021. Nghị định này thay thế Nghị định số 1/2020/NĐ-CP ngày 1/1/2020 của Chính phủ.")
	if err := loadValidityFixture(t, f, nil); err != nil {
		t.Fatal(err)
	}
	var end, class string
	if err := f.corpus.QueryRow(context.Background(), `SELECT eff_to::text,status_class FROM document WHERE key='vn:vbpl:1'`).Scan(&end, &class); err != nil || end != "2021-02-01" || class != "in_force" {
		t.Fatalf("%s %s: %v", end, class, err)
	}
}
func TestLoad_ValidityRuleCMarksConflict(t *testing.T) {
	f := fixture(t)
	addReplacementFixture(t, f, "Nghị định này có hiệu lực từ ngày 1/2/2021.")
	if err := loadValidityFixture(t, f, nil); err != nil {
		t.Fatal(err)
	}
	var conflict bool
	if err := f.corpus.QueryRow(context.Background(), `SELECT validity_conflict FROM document WHERE key='vn:vbpl:1'`).Scan(&conflict); err != nil || !conflict {
		t.Fatalf("%v: %v", conflict, err)
	}
}
func correctionFixture() source.Correction {
	status := "expired"
	return source.Correction{Key: "vn:vbpl:1", Number: "1/2024/NĐ-CP", Set: source.CorrectionSet{StatusClass: &status}, Reason: "test", Evidence: source.CorrectionEvidence{Anchor: "dieu-1", Quote: "Nghị định này có hiệu lực"}, Reviewed: "2026-11-10"}
}
func TestLoad_ManualCorrectionWinsAndIsRecorded(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CCHL", "2024-01-01", "2027-01-01", "Nghị định này có hiệu lực từ ngày 1/7/2024.")
	if err := loadValidityFixture(t, f, []source.Correction{correctionFixture()}); err != nil {
		t.Fatal(err)
	}
	var class, basis string
	var n int
	if err := f.corpus.QueryRow(context.Background(), `SELECT status_class,status_basis,(SELECT count(*) FROM validity_decision WHERE layer='manual') FROM document`).Scan(&class, &basis, &n); err != nil || class != "expired" || basis != "manual_correction" || n != 1 {
		t.Fatalf("%s %s %d: %v", class, basis, n, err)
	}
}
func TestLoad_RejectedCorrectionFailsBuild(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CHL", "2024-01-01", "2024-01-01", "")
	c := correctionFixture()
	c.Number = "wrong"
	if err := loadValidityFixture(t, f, []source.Correction{c}); err == nil || !strings.Contains(err.Error(), c.Key) {
		t.Fatalf("error %v", err)
	}
}
func TestLoad_DecisionRowsOrdered(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CCHL", "2024-01-01", "2027-01-01", "Nghị định này có hiệu lực từ ngày 1/7/2024.")
	if err := loadValidityFixture(t, f, []source.Correction{correctionFixture()}); err != nil {
		t.Fatal(err)
	}
	var layers []string
	if err := f.corpus.QueryRow(context.Background(), `SELECT array_agg(layer ORDER BY ordinal) FROM validity_decision`).Scan(&layers); err != nil || fmt.Sprint(layers) != "[source A manual]" {
		t.Fatalf("layers %v: %v", layers, err)
	}
}

func TestLoad_FutureActorNotStartedAtReleaseDate(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2020/NĐ-CP", "CHL", "2020-01-01", "2020-01-01", "")
	hash := addValidityFixture(t, f, "2", "2/2100/NĐ-CP", "CHL", "2100-01-01", "2100-02-01", "")
	f.addArtifact(t, "relate", "vn:vbpl:2", map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "replaces", "target_vbpl_id": "1"}}}, f.relateInput(t, "vn:vbpl:2", hash))
	if err := loadValidityFixture(t, f, nil); err != nil {
		t.Fatal(err)
	}
	var conflict bool
	if err := f.corpus.QueryRow(context.Background(), `SELECT validity_conflict FROM document WHERE key='vn:vbpl:1'`).Scan(&conflict); err != nil || conflict {
		t.Fatalf("conflict=%v: %v", conflict, err)
	}
}
