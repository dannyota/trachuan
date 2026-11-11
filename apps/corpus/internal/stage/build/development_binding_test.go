package build

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/internal/corpusdb"
)

func bindingArtifact(t *testing.T, binding *string, oldRecipe bool) (artifact.Store, artifact.Result, stage.Run, Config, string) {
	t.Helper()
	cfg := Config{Suite: evaluation.Suite{Hashes: evaluation.FixtureHashes{IdentityMap: "map", Development: "current-fixture"}}}
	run := stage.Run{Country: "vn", Rel: "20990101", Snapshot: stage.Snapshot{SourceListHash: "sources", SourceList: []byte(`{"country":"vn","status_codes":{}}`), WorkerVersion: "worker"}}
	inv := Inventory{Country: "vn", SourceListHash: "sources"}
	input := New(cfg).inputHash(run.Country, run.Rel, inv, correctionInput{})
	recipe := New(cfg).Recipe(run.Snapshot)
	rowRecipe := recipe
	if oldRecipe {
		rowRecipe = strings.Repeat("b", 64)
	}
	dump := []byte("synthetic dump")
	calibration := []byte(`{"threshold":0}`)
	record := map[string]any{
		"version": 1, "country": run.Country, "rel": run.Rel, "database_name": "corpus_vn_20990101", "schema_version": corpusdb.SchemaVersion,
		"build_input_hash": input, "build_recipe_hash": rowRecipe, "source_manifest_hash": inv.SourceManifestHash(), "identity_inventory_hash": inv.IdentityHash(), "identity_map_sha256": "map",
		"worker_version": "worker", "built_at": time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		"dump":             map[string]any{"sha256": hashBytes(dump), "bytes": len(dump), "format": "custom"},
		"retrieval_recipe": map[string]any{"abstention": map[string]any{"threshold": 0, "calibration_sha256": hashBytes(calibration)}},
	}
	if binding != nil {
		record["development_cases_sha256"] = *binding
	}
	files := map[string]any{"build.json": record, "inputs.json": inv, "calibration.json": json.RawMessage(calibration), "validity-review.json": json.RawMessage(`{"version":1,"entries":[]}`)}
	artifacts := artifact.Store{Root: t.TempDir()}
	art, err := artifacts.Publish(context.Background(), "build", input, rowRecipe, func(dir string) error {
		for name, value := range files {
			data, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				return err
			}
		}
		return os.WriteFile(filepath.Join(dir, "corpus.dump"), dump, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifacts, art, run, cfg, rowRecipe
}

func TestVerifyBuildArtifact_DevelopmentBinding(t *testing.T) {
	for _, tc := range []struct {
		name, binding string
		missing       bool
		want          string
	}{
		{name: "predates binding", missing: true, want: "build predates development fixture binding and must be rebuilt"},
		{name: "different binding", binding: "different-fixture", want: "build artifact identity mismatch"},
		{name: "current binding", binding: "current-fixture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var binding *string
			if !tc.missing {
				binding = &tc.binding
			}
			artifacts, art, run, cfg, _ := bindingArtifact(t, binding, tc.missing)
			_, dump, err := verifyBuildArtifact(artifacts, art, run, cfg)
			if tc.want != "" {
				if err == nil || err.Error() != tc.want {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(dump)
				if err != nil || string(data) != "synthetic dump" {
					t.Fatalf("verified dump: %v", err)
				}
			}
		})
	}
}

func TestVerifyForRun_DevelopmentBinding(t *testing.T) {
	for _, tc := range []struct {
		name, binding string
		missing       bool
		rowMismatch   bool
		want          string
	}{
		{name: "predates binding", missing: true, want: "build predates development fixture binding and must be rebuilt"},
		{name: "different binding", binding: "different-fixture", want: "build artifact identity mismatch"},
		{name: "current binding", binding: "current-fixture"},
		{name: "committed row recipe mismatch", binding: "current-fixture", rowMismatch: true, want: "invalid committed build artifact"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var binding *string
			if !tc.missing {
				binding = &tc.binding
			}
			artifacts, art, run, cfg, rowRecipe := bindingArtifact(t, binding, tc.missing)
			if tc.rowMismatch {
				rowRecipe = strings.Repeat("c", 64)
			}
			raw, err := json.Marshal(map[string]any{"items": []any{map[string]any{"stage": "build", "item_key": "vn/20990101", "state": "done", "path": art.Path, "input_hash": art.InputHash, "recipe_hash": rowRecipe, "artifact_sha256": art.SHA256}}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := VerifyForRun(context.Background(), cfg, inventoryQueryStub{raw: raw}, artifacts, run)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
			} else if err != nil || got.Path != art.Path {
				t.Fatalf("committed artifact = %+v, error = %v", got, err)
			}
		})
	}
}
