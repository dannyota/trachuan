//go:build integration

package build

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func prepareOmittedSummary(t *testing.T, f *loadFixture, rowCap bool) string {
	t.Helper()
	ctx := context.Background()
	key, source, class := "vn:vbpl:2", "vbpl", "primary"
	if rowCap {
		key, source, class = "vn:chinhphu:2", "chinhphu", "secondary"
		url := "https://example.test/" + strings.Repeat("a", 65536)
		html := []byte("detail")
		metadata, err := json.Marshal(map[string]any{"version": 1, "source_id": "2", "source_url": url, "occurrences": []any{}, "attachments": []any{}})
		if err != nil {
			t.Fatal(err)
		}
		document := map[string]any{"version": 2, "document": map[string]any{
			"key": key, "country": "vn", "source": source, "source_id": "2", "number": "2", "title": "Title", "source_url": url,
			"identity_evidence": []any{map[string]any{"path": "detail.html", "sha256": hashBytes(html)}, map[string]any{"path": "metadata.json", "sha256": hashBytes(metadata)}},
		}}
		f.addArtifact(t, "fetch", key, map[string]any{"document.json": document, "metadata.json": metadata, "detail.html": html, "files.json": map[string]any{"version": 2, "files": []any{}}})
	} else {
		f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": "2", "docNum": strings.Repeat("ố", 201)}}, "files.json": []any{}})
	}
	f.run.Snapshot.SourceList = []byte(`{"country":"vn","vbpl":{"selection_version":1},"status_codes":{}}`)
	for _, entry := range []struct{ key, stage string }{{"vn:vbpl:1", "relate"}, {key, "extract"}, {key, "relate"}} {
		if _, err := f.workshop.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,$2,$3,'gap',0)`, f.run.ID, entry.stage, entry.key); err != nil {
			t.Fatal(err)
		}
	}
	entries := []membership.Entry{
		{DocumentKey: "vn:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"},
		{DocumentKey: key, Source: source, SourceID: "2", Class: class, Disposition: "active"},
	}
	if err := membership.PublishPhase(ctx, store.New(f.workshop), f.run.ID, membership.Final, entries); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestBuild_SummaryOmissionsSurviveResolveAndRestore(t *testing.T) {
	for _, rowCap := range []bool{false, true} {
		name, kind := "identity_field", "summary_field_capped"
		if rowCap {
			name, kind = "row", "summary_row_capped"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var key string
			f := recoverySetup(t, func(input *loadFixture) { key = prepareOmittedSummary(t, input, rowCap) })
			record, _, err := verifyBuildArtifact(f.env.Artifacts, f.art, f.run, f.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if record.Counts["document"] != 2 || record.Counts["document_summary"] != 1 || record.Counts["document_summary_omitted"] != 1 {
				t.Fatalf("summary counts=%v", record.Counts)
			}
			data, err := os.ReadFile(filepath.Join(f.env.Artifacts.Abs(f.art.Path), "heading-scan.json"))
			if err != nil || !strings.Contains(string(data), `"omitted":`) {
				t.Fatalf("heading omission report missing: %v", err)
			}
			result, err := New(f.cfg).Resolve(ctx, f.env, f.run, f.item, f.art)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, gap := range result.Gaps {
				found = found || (gap.Kind == kind && strings.Contains(gap.Detail, key))
			}
			if !found {
				t.Fatalf("missing omission gap: %+v", result.Gaps)
			}
			built, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
			if err != nil || built.Instance != "built" {
				t.Fatalf("resolve built database: %v", err)
			}
			if !testpg.DropCorpusDatabase(t, f.admin, built.Name, "vn/"+f.run.Rel) {
				t.Fatal("built database was not dropped")
			}
			restored, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
			if err != nil || restored.Instance != "restored" {
				t.Fatalf("restore: %v", err)
			}
			db, err := pgx.Connect(ctx, restored.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close(ctx) }()
			var summaries, omitted int
			if err = db.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE document_key=$1) FROM document_summary`, key).Scan(&summaries, &omitted); err != nil || summaries != 1 || omitted != 0 {
				t.Fatalf("restored summaries=%d omitted row=%d: %v", summaries, omitted, err)
			}
			if _, err = db.Exec(ctx, `UPDATE coverage_gap SET detail='topic.name' WHERE document_key=$1 AND kind=$2`, key, kind); err != nil {
				t.Fatal(err)
			}
			if _, err = EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run); err == nil || !strings.Contains(err.Error(), "summary document mismatch") {
				t.Fatalf("accepted omission without row-omitting detail: %v", err)
			}
			detail := "number"
			if rowCap {
				detail = ""
			}
			if _, err = db.Exec(ctx, `UPDATE coverage_gap SET detail=$3 WHERE document_key=$1 AND kind=$2`, key, kind, detail); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `UPDATE coverage_gap SET stage='extract' WHERE document_key=$1 AND kind=$2`, key, kind); err != nil {
				t.Fatal(err)
			}
			if _, err = EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run); err == nil || !strings.Contains(err.Error(), "summary document mismatch") {
				t.Fatalf("accepted omission without build gap: %v", err)
			}
			if _, err = db.Exec(ctx, `UPDATE coverage_gap SET stage='build' WHERE document_key=$1 AND kind=$2`, key, kind); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `UPDATE document_summary SET status_basis='manual_correction' WHERE document_key='vn:vbpl:1'`); err != nil {
				t.Fatal(err)
			}
			if _, err = EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run); err == nil || !strings.Contains(err.Error(), "summary document mismatch") {
				t.Fatalf("accepted mismatched retained summary: %v", err)
			}
			if _, err = db.Exec(ctx, `UPDATE document_summary SET status_basis='source_status',class='secondary',indexed=false WHERE document_key='vn:vbpl:1'`); err != nil {
				t.Fatal(err)
			}
			if _, err = EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run); err == nil || !strings.Contains(err.Error(), "summary document mismatch") {
				t.Fatalf("accepted mismatched retained class: %v", err)
			}
		})
	}
}
