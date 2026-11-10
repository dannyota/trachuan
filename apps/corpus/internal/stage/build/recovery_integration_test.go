//go:build integration

package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recoveryFixture struct {
	cfg   Config
	env   stage.Env
	run   stage.Run
	item  stage.Item
	art   artifact.Result
	admin *pgx.Conn
}

func recoverySetup(t *testing.T) recoveryFixture {
	t.Helper()
	ctx := context.Background()
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", false)
	builder := testpg.NewBuilder(t)
	pool, err := pgxpool.New(ctx, f.workshop.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
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
	admin, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	temp := "corpus_tmp_vn_" + items[0].InputHash[:24]
	final := "corpus_vn_" + f.run.Rel
	t.Cleanup(func() {
		testpg.DropCorpusDatabase(t, admin, temp, "vn/"+f.run.Rel)
		testpg.DropCorpusDatabase(t, admin, final, "vn/"+f.run.Rel)
	})
	output, err := st.Process(ctx, env, f.run, items[0])
	if err != nil || output.Artifact == nil {
		t.Fatalf("build: %v", err)
	}
	return recoveryFixture{cfg: cfg, env: env, run: f.run, item: items[0], art: *output.Artifact, admin: admin}
}

func TestResolve_FreshBuildBeforeCommit(t *testing.T) {
	f := recoverySetup(t)
	if _, err := New(f.cfg).Resolve(context.Background(), f.env, f.run, f.item, f.art); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDatabase(context.Background(), f.cfg, f.env.Artifacts, f.art, f.run); err != nil {
		t.Fatal(err)
	}
}

func TestResolve_RestoresLostDatabaseFromDump(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	built, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if !testpg.DropCorpusDatabase(t, f.admin, built.Name, "vn/"+f.run.Rel) {
		t.Fatal("database was not dropped")
	}
	restored, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
	if err != nil || restored.Instance != "restored" {
		t.Fatalf("restore: %s %v", restored.Instance, err)
	}
	again, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
	if err != nil || again.Instance != "restored" {
		t.Fatalf("restored instance lost: %s %v", again.Instance, err)
	}
}

func TestEnsureDatabase_IdempotentAfterEvaluation(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	one, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
	if err != nil {
		t.Fatal(err)
	}
	two, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
	if err != nil || one.Name != two.Name {
		t.Fatalf("repeat: %v", err)
	}
}

func TestResolve_MismatchedFinalDatabaseIsError(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	built, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
	if err != nil {
		t.Fatal(err)
	}
	marker, err := readMarker(ctx, f.admin, built.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{built.Name}.Sanitize()+" IS 'foreign'")
	if err != nil {
		t.Fatal(err)
	}
	_, resolveErr := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run)
	if err = writeMarker(ctx, f.admin, built.Name, marker); err != nil {
		t.Fatal(err)
	}
	if resolveErr == nil {
		t.Fatal("accepted mismatched final database")
	}
}

func TestResolve_CrashBeforeRenameRecreatesOnlyOwnedTemporary(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	temp := "corpus_tmp_vn_" + f.item.InputHash[:24]
	if _, err := f.admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{temp}.Sanitize()+" IS NULL"); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_CalibrationRecordedInRecipe(t *testing.T) {
	f := recoverySetup(t)
	record, _, err := verifyBuildArtifact(f.env.Artifacts, f.art, f.run, f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	var recipe struct {
		Abstention struct {
			Threshold         *float64 `json:"threshold"`
			CalibrationSHA256 string   `json:"calibration_sha256"`
		} `json:"abstention"`
	}
	if err = json.Unmarshal(record.RetrievalRecipe, &recipe); err != nil || recipe.Abstention.Threshold == nil || len(recipe.Abstention.CalibrationSHA256) != 64 {
		t.Fatalf("calibration: %v", err)
	}
}

func TestBuild_EmptyTaxonomyTables(t *testing.T) {
	f := recoverySetup(t)
	record, _, err := verifyBuildArtifact(f.env.Artifacts, f.art, f.run, f.cfg)
	if err != nil || record.Counts["taxonomy_node"] != 0 || record.Counts["taxonomy_assignment"] != 0 {
		t.Fatal("taxonomy is not empty", err)
	}
}

func TestBuild_HostileNamesRejected(t *testing.T) {
	for _, name := range []string{"20260101;DROP", "20260101_999", "../../x"} {
		if _, err := DatabaseName("vn", name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestBuild_LockSerializesConcurrentOperations(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	if err := lock(ctx, f.admin, f.run.Country, f.run.Rel); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := EnsureDatabase(ctx, f.cfg, f.env.Artifacts, f.art, f.run); results <- err }()
	}
	select {
	case err := <-results:
		t.Fatalf("operation passed held lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := unlock(ctx, f.admin, f.run.Country, f.run.Rel); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuild_CancellationStopsCopy(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", false)
	inv := f.inventory(t)
	if _, err := f.corpus.Exec(context.Background(), `CREATE FUNCTION slow_chunk() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(10); RETURN NEW; END $$; CREATE TRIGGER slow_chunk BEFORE INSERT ON chunk FOR EACH ROW EXECUTE FUNCTION slow_chunk()`); err != nil {
		t.Fatal(err)
	}
	observer, err := pgx.ConnectConfig(context.Background(), f.corpus.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observer.Close(context.Background()) }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, loadErr := Load(ctx, f.corpus, inv, f.artifacts, f.recipes, nil); done <- loadErr }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting bool
		if err = observer.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event='PgSleep')`, f.corpus.PgConn().PID()).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("load did not reach chunk insertion")
		}
		select {
		case loadErr := <-done:
			t.Fatalf("load ended before cancellation: %v", loadErr)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err = <-done; err == nil {
		t.Fatal("cancelled load succeeded")
	}
	var count int
	if err = observer.QueryRow(context.Background(), `SELECT count(*) FROM chunk`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial chunks %d: %v", count, err)
	}
}

func TestRestore_DropsExtensionEntriesAndRefusesFunctions(t *testing.T) {
	list := "1; 3079 1 EXTENSION - vector owner\n2; 1259 2 TABLE public chunk owner\n"
	filtered, err := filterRestoreList(list)
	if err != nil || strings.Contains(filtered, "EXTENSION") {
		t.Fatalf("extension filtering: %v", err)
	}
	if _, err = filterRestoreList("3; 1255 3 FUNCTION public evil() owner\n"); err == nil {
		t.Fatal("accepted function")
	}
}

func TestEnsureForRun_UsesCommittedBuild(t *testing.T) {
	f := recoverySetup(t)
	ctx := context.Background()
	var id int64
	err := f.env.DB.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES('build',$1,$2,$3,$4,$5) RETURNING id`, f.item.Key, f.item.InputHash, New(f.cfg).Recipe(f.run.Snapshot), f.art.Path, f.art.SHA256).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.env.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,input_hash,recipe_hash,artifact_id) VALUES($1,'build',$2,'done',0,$3,$4,$5)`, f.run.ID, f.item.Key, f.item.InputHash, New(f.cfg).Recipe(f.run.Snapshot), id)
	if err != nil {
		t.Fatal(err)
	}
	db, err := EnsureForRun(ctx, f.cfg, store.New(f.env.DB), f.env.Artifacts, f.run)
	if err != nil || db.Name != "corpus_vn_"+f.run.Rel {
		t.Fatalf("resolve committed build: %v", err)
	}
}

func TestBuild_LostLockConnectionCancelsWork(t *testing.T) {
	ctx := context.Background()
	builder := testpg.NewBuilder(t)
	holder, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close(ctx) }()
	other, err := pgx.Connect(ctx, builder.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close(ctx) }()
	if err = lock(ctx, holder, "vn", testpg.Release(t)); err != nil {
		t.Fatal(err)
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := watchLock(work, holder, cancel, nil)
	var killed bool
	if err = other.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, holder.PgConn().PID()).Scan(&killed); err != nil || !killed {
		t.Fatalf("terminate lock: %v", err)
	}
	select {
	case <-work.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("lost lock did not cancel work")
	}
	if err = stop(); err == nil {
		t.Fatal("lost lock reported success")
	}
	if err = lock(ctx, other, "vn", testpg.Release(t)); err != nil {
		t.Fatal(err)
	}
	if err = unlock(ctx, other, "vn", testpg.Release(t)); err != nil {
		t.Fatal(err)
	}
}

func TestResolve_ReturnsBuildGaps(t *testing.T) {
	f := recoverySetup(t)
	result, err := New(f.cfg).Resolve(context.Background(), f.env, f.run, f.item, f.art)
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range result.Gaps {
		if gap.Kind == "unknown_status" && strings.Contains(gap.Detail, "vn:vbpl:1") {
			return
		}
	}
	t.Fatal("missing document status gap")
}
