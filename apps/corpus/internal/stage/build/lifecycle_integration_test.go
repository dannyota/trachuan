//go:build integration

package build

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk/normalizer"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuild_RefusesPlanWhilePredecessorPendingOrFailed(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, f.workshop.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	env := stage.Env{DB: pool, Artifacts: f.artifacts}
	for _, state := range []string{"pending", "failed"} {
		if _, err = f.workshop.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,'embed','vn:vbpl:1',$2,0) ON CONFLICT (run_id,stage,item_key) DO UPDATE SET state=$2`, f.run.ID, state); err != nil {
			t.Fatal(err)
		}
		if _, err = New(Config{}).Plan(ctx, env, f.run); err == nil {
			t.Fatalf("planned with %s predecessor", state)
		}
	}
}

func TestBuild_TemporaryDatabaseMarker(t *testing.T) {
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	admin, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	name := "corpus_tmp_vn_0123456789abcdef01234567"
	marker := testOwnershipMarker(t, "a", "built")
	cfg := Config{Template: builder.Template}
	if err = recreateTemporary(ctx, admin, cfg, name, marker); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		testpg.DropCorpusDatabase(t, admin, name, "vn/"+testpg.Release(t))
	})
	got, err := readMarker(ctx, admin, name)
	if err != nil || got != marker {
		t.Fatalf("marker %q: %v", got, err)
	}
	if err = recreateTemporary(ctx, admin, cfg, name, testOwnershipMarker(t, "c", "built")); err == nil {
		t.Fatal("foreign marker accepted")
	}
	if err = recreateTemporary(ctx, admin, cfg, name, marker); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_LockAndRename(t *testing.T) {
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	admin, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	if err = lock(ctx, admin, "vn", testpg.Release(t)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlock(ctx, admin, "vn", testpg.Release(t)) }()
	temp := "corpus_tmp_vn_aaaaaaaaaaaaaaaaaaaaaaaa"
	final := "corpus_vn_" + testpg.Release(t)
	t.Cleanup(func() {
		testpg.DropCorpusDatabase(t, admin, temp, "vn/"+testpg.Release(t))
		testpg.DropCorpusDatabase(t, admin, final, "vn/"+testpg.Release(t))
	})
	if err = recreateTemporary(ctx, admin, Config{Template: builder.Template}, temp, testOwnershipMarker(t, "a", "built")); err != nil {
		t.Fatal(err)
	}
	if err = renameToFinal(ctx, admin, temp, final); err != nil {
		t.Fatal(err)
	}
	marker, err := readMarker(ctx, admin, final)
	if err != nil || marker != testOwnershipMarker(t, "a", "built") {
		t.Fatalf("renamed marker %q: %v", marker, err)
	}
}

func TestBuild_DumpCreatesArchive(t *testing.T) {
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	path := filepath.Join(t.TempDir(), "corpus.dump")
	if err := pgDump(ctx, Config{BuilderURL: builder.URL, PgDump: testTool("CORPUS_PG_DUMP", "pg_dump")}, builder.Template, path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("dump: %v", err)
	}
}

func TestBuild_ProcessCreatesTemporaryRelease(t *testing.T) {
	ctx := context.Background()
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", false)
	builder := testpg.NewBuilder(t)
	pool, err := pgxpool.New(ctx, f.workshop.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	f.run.Snapshot.SourceList = []byte(`{"country":"vn","status_codes":{}}`)
	inv := f.inventory(t)
	suite := evaluation.Suite{Legacy: []evaluation.Case{{ID: "one", Question: "ngân hàng", ExpectAbstain: true}, {ID: "two", Question: "abc", ExpectAbstain: true}, {ID: "three", Question: "def", ExpectAbstain: true}, {ID: "four", Question: "ghi", ExpectAbstain: true}}, Rejections: []string{"two", "three", "four"}, Hashes: evaluation.FixtureHashes{Legacy: "legacy"}, Map: evaluation.IdentityMap{Version: 1, LegacySHA256: "legacy", IdentityInventoryHash: inv.IdentityHash()}}
	pinTestSuite(t, &suite)
	cfg := Config{BuilderURL: builder.URL, Template: builder.Template, PgDump: testTool("CORPUS_PG_DUMP", "pg_dump"), PgRestore: testTool("CORPUS_PG_RESTORE", "pg_restore"), Suite: suite, Now: time.Now, Embedder: buildFakeEmbedder{}, DatabaseDiskPath: t.TempDir(), MinFreeBeforeBytes: 1, MinFreeDuringBytes: 1}
	cfg.Versions, err = InspectVersions(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	st := New(cfg)
	env := stage.Env{DB: pool, Artifacts: f.artifacts}
	items, err := st.Plan(ctx, env, f.run)
	if err != nil {
		t.Fatal(err)
	}
	temp := "corpus_tmp_vn_" + items[0].InputHash[:24]
	admin, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, temp, "vn/"+f.run.Rel) })
	out, err := st.Process(ctx, env, f.run, items[0])
	if err != nil || out.Artifact == nil {
		t.Fatalf("process: %v", err)
	}
	dbCfg := admin.Config().Copy()
	dbCfg.Database = temp
	db, err := pgx.ConnectConfig(ctx, dbCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close(ctx) }()
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM release`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("release rows %d: %v", count, err)
	}
}

type buildFakeEmbedder struct{}

func (buildFakeEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (buildFakeEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	values := make([]float32, 1024)
	values[0] = 1
	return embedding.Vector{Values: values, TokenCount: 1, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}

func testTool(env, fallback string) string {
	if value := os.Getenv(env); value != "" {
		return value
	}
	return fallback
}

func TestEnsureForRun_RequiresDoneBuild(t *testing.T) {
	f := fixture(t)
	if _, err := EnsureForRun(context.Background(), Config{}, store.New(f.workshop), f.artifacts, f.run); err == nil {
		t.Fatal("accepted a run without a completed build")
	}
}

func testOwnershipMarker(t *testing.T, inputChar, instance string) string {
	t.Helper()
	marker, err := encodeOwnershipMarker(ownershipMarker{RunKey: "vn/" + testpg.Release(t), InputHash: strings.Repeat(inputChar, 64), RecipeHash: strings.Repeat("b", 64), Instance: instance})
	if err != nil {
		t.Fatal(err)
	}
	return marker
}

func TestBuild_RestoredMarkerPreservesOwnership(t *testing.T) {
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	admin, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	name := "corpus_tmp_vn_bbbbbbbbbbbbbbbbbbbbbbbb"
	rel := testpg.Release(t)
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, "vn/"+rel) })
	built := testOwnershipMarker(t, "a", "built")
	if err = recreateTemporary(ctx, admin, Config{Template: builder.Template}, name, built); err != nil {
		t.Fatal(err)
	}
	if err = markRestored(ctx, admin, name); err != nil {
		t.Fatal(err)
	}
	raw, err := readMarker(ctx, admin, name)
	if err != nil || !sameOwnership(built, raw) {
		t.Fatalf("ownership changed: %v", err)
	}
	kind, err := databaseInstance(ctx, admin, name)
	if err != nil || kind != "restored" {
		t.Fatalf("instance %q: %v", kind, err)
	}
	if err = recreateTemporary(ctx, admin, Config{Template: builder.Template}, name, built); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_FrozenSchema2RecipeRefused(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, f.workshop.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	snapshot := stage.Snapshot{SourceList: []byte(`{"country":"vn","status_codes":{}}`)}
	old := buildRecipe{SchemaVersion: 2, SchemaSHA256: "3aa0edadd92150db2ddbb4eb17ad47e55e2e91db3c872a8488a9b6488843aff6", ChunkRecipeHash: chunk.Recipe2().Hash(), DocumentRecipeHash: embedding.DocumentRecipe1().Hash(), QueryRecipeHash: embedding.QueryRecipe1().Hash(), LexicalRecipeHash: lexical.Recipe1().Hash(), RetrievalPolicyHash: retrieval.Recipe2().Hash(), NormalizerVersion: normalizer.Version, NumberNormalizationVersion: retrieval.NumberNormalizationVersion, StatusCodesSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("{}"))), CalibrationPolicy: "provisional-old-product/1"}
	snapshot.Recipes = map[string]string{"build": old.hash()}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, updateErr := f.workshop.Exec(ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, f.run.ID, data); updateErr != nil {
		t.Fatal(updateErr)
	}
	compiled := New(Config{}).Recipe(snapshot)
	if compiled == "" || compiled == old.hash() {
		t.Fatal("invalid compiled recipe")
	}
	_, err = stage.PlanStage(ctx, stage.Env{DB: pool}, New(Config{}), stage.StageInput{RunID: f.run.ID, Stage: "build"}, map[string]string{"build": compiled})
	if err == nil || !strings.Contains(err.Error(), "recipe mismatch for stage build") {
		t.Fatalf("error = %v", err)
	}
}
