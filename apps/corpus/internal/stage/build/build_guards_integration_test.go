//go:build integration

package build

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type temporaryState struct {
	oid                             uint32
	marker                          string
	artifactSHA256                  string
	inventorySHA256                 string
	documents, chunks               int64
	releaseIdentity, chunkIdentity  string
	artifactRows, itemRows, gapRows int64
}

func inspectTemporary(t *testing.T, f recoveryFixture) temporaryState {
	t.Helper()
	ctx := context.Background()
	name := "corpus_tmp_" + f.run.Country + "_" + f.item.InputHash[:24]
	var state temporaryState
	if err := f.admin.QueryRow(ctx, `SELECT oid FROM pg_database WHERE datname=$1`, name).Scan(&state.oid); err != nil {
		t.Fatal(err)
	}
	var err error
	state.marker, err = readMarker(ctx, f.admin, name)
	if err != nil {
		t.Fatal(err)
	}
	state.artifactSHA256, err = artifact.Verify(f.env.Artifacts.Abs(f.art.Path))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := LoadInventory(ctx, store.New(f.env.DB), f.run, "embed")
	if err != nil {
		t.Fatal(err)
	}
	state.inventorySHA256 = inv.SourceManifestHash()
	if state.artifactSHA256 != f.art.SHA256 {
		t.Fatal("published artifact hash changed")
	}
	if err = f.env.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM artifact), (SELECT count(*) FROM run_item WHERE run_id=$1), (SELECT count(*) FROM gap WHERE run_id=$1)`, f.run.ID).Scan(&state.artifactRows, &state.itemRows, &state.gapRows); err != nil {
		t.Fatal(err)
	}
	cfg := f.admin.Config().Copy()
	cfg.Database = name
	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(ctx) }()
	if err = db.QueryRow(ctx, `SELECT (SELECT count(*) FROM document), (SELECT count(*) FROM chunk), (SELECT build_input_hash || ':' || build_recipe_hash FROM release WHERE singleton), (SELECT COALESCE(string_agg(input_sha256, ',' ORDER BY id), '') FROM chunk)`).Scan(&state.documents, &state.chunks, &state.releaseIdentity, &state.chunkIdentity); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestBuild_IdentityInventoryHashDoesNotGatePlanning(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	if _, err := New(f.cfg).Plan(ctx, f.env, f.run); err != nil {
		t.Fatalf("valid map: %v", err)
	}
	cfg := f.cfg
	cfg.Suite.Map.IdentityInventoryHash = strings.Repeat("f", 64)
	pinTestSuite(t, &cfg.Suite)
	if cfg.Suite.Map.IdentityInventoryHash == f.cfg.Suite.Map.IdentityInventoryHash {
		t.Fatal("test did not change inventory identity")
	}
	if _, err := New(cfg).Plan(ctx, f.env, f.run); err != nil {
		t.Fatalf("diagnostic inventory hash blocked plan: %v", err)
	}
}

func TestBuild_ForeignTemporaryDatabaseNotAdopted(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	name := "corpus_tmp_" + f.run.Country + "_" + f.item.InputHash[:24]
	raw, err := readMarker(ctx, f.admin, name)
	if err != nil {
		t.Fatal(err)
	}
	marker, err := parseOwnershipMarker(raw)
	if err != nil {
		t.Fatal(err)
	}
	marker.RecipeHash = strings.Repeat("f", 64)
	if marker.RecipeHash == New(f.cfg).Recipe(f.run.Snapshot) {
		marker.RecipeHash = strings.Repeat("e", 64)
	}
	foreign, err := encodeOwnershipMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeMarker(ctx, f.admin, name, foreign); err != nil {
		t.Fatal(err)
	}
	before := inspectTemporary(t, f)
	out, err := New(f.cfg).Process(ctx, f.env, f.run, f.item)
	if err == nil || !strings.Contains(err.Error(), "temporary database ownership mismatch") || out.Artifact != nil {
		t.Fatalf("foreign database accepted: %v", err)
	}
	after := inspectTemporary(t, f)
	if before != after || after.marker != foreign {
		t.Fatal("foreign database or build evidence changed")
	}
}

func TestBuild_StaleGenerationCannotCommit(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	if _, err := f.env.DB.Exec(ctx, `UPDATE run SET generation=generation+1 WHERE id=$1`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	before := inspectTemporary(t, f)
	out, err := New(f.cfg).Process(ctx, f.env, f.run, f.item)
	if !errors.Is(err, stage.ErrSuperseded) || out.Artifact != nil {
		t.Fatalf("stale generation: %v", err)
	}
	if after := inspectTemporary(t, f); before != after {
		t.Fatal("stale attempt changed database or evidence")
	}
}

func TestBuild_DumpMatchesBuildRecord(t *testing.T) {
	f := recoverySetup(t)
	dir := f.env.Artifacts.Abs(f.art.Path)
	data, err := os.ReadFile(filepath.Join(dir, "build.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Dump struct {
			SHA256 string `json:"sha256"`
			Bytes  int64  `json:"bytes"`
			Format string `json:"format"`
		} `json:"dump"`
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	dump, err := os.Open(filepath.Join(dir, "corpus.dump"))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	n, err := io.Copy(hash, dump)
	closeErr := dump.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if n <= 0 || record.Dump.Bytes != n || record.Dump.SHA256 != fmt.Sprintf("%x", hash.Sum(nil)) || record.Dump.Format != "custom" {
		t.Fatal("dump does not match build record")
	}
}

func TestBuild_InventoryChangedFails(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	result, err := f.env.DB.Exec(ctx, `UPDATE run_item SET recipe_hash=$3 WHERE run_id=$1 AND stage='embed' AND item_key=$2`, f.run.ID, "vn:vbpl:1", strings.Repeat("e", 64))
	if err != nil || result.RowsAffected() != 1 {
		t.Fatalf("change upstream item: %v", err)
	}
	before := inspectTemporary(t, f)
	out, err := New(f.cfg).Process(ctx, f.env, f.run, f.item)
	if err == nil || !strings.Contains(err.Error(), "inventory_changed") || out.Artifact != nil {
		t.Fatalf("inventory change: %v", err)
	}
	if after := inspectTemporary(t, f); before != after {
		t.Fatal("inventory change altered database or build evidence")
	}
}
