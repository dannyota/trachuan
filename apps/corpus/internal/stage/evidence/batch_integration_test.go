//go:build integration

package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/detector"
	corpusrun "danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

const batchKey = "vn/" + testRel

type batchFixture struct {
	t       *testing.T
	ctx     context.Context
	db      *pgxpool.Pool
	env     stage.Env
	stage   *Stage
	run     stage.Run
	recipes map[string]string
	build   artifact.Result
	detect  func(context.Context) error
	calls   atomic.Int32
}

type batchDetector struct{ fixture *batchFixture }

func (batchDetector) Spec() detector.Detector {
	return detector.Detector{ID: "batch_check", Version: 1, Semantics: 1, Severity: detector.Review}
}

func (d batchDetector) Detect(ctx context.Context, _ Inputs) (Observation, error) {
	d.fixture.calls.Add(1)
	if d.fixture.detect != nil {
		if err := d.fixture.detect(ctx); err != nil {
			return Observation{}, err
		}
	}
	return Observation{Observed: 1, ObservedCompletely: always}, nil
}

type crashAfterRename struct {
	*Stage
	cancel context.CancelFunc
}

func (c crashAfterRename) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	c.cancel()
	return nil, context.Canceled
}

type batchTemporal struct{ client.Client }

func (batchTemporal) DescribeWorkflowExecution(context.Context, string, string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return nil, serviceerror.NewNotFound("no workflow")
}

func (batchTemporal) ExecuteWorkflow(context.Context, client.StartWorkflowOptions, any, ...any) (client.WorkflowRun, error) {
	return nil, nil
}

func newBatchFixture(t *testing.T, decision stage.PreviousReport) *batchFixture {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	f := &batchFixture{t: t, ctx: ctx, db: db, env: stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}}
	f.stage = New(Config{Detectors: []Detector{batchDetector{fixture: f}}})
	f.stage.release = func(context.Context, stage.Env, stage.Run) (*pgxpool.Pool, func(), error) { return nil, func() {}, nil }
	f.stage.lookup = func(context.Context, stage.Env, store.Querier, stage.Run) (artifact.Result, error) {
		return f.build, nil
	}
	f.recipes = map[string]string{"fetch": strings.Repeat("3", 64), "build": strings.Repeat("4", 64), "eval": strings.Repeat("5", 64), stage.EvidenceStage: f.stage.Recipe(stage.Snapshot{})}
	snapshot := stage.Snapshot{SourceList: json.RawMessage(`{}`), Stages: []string{"fetch", "build", "eval", stage.EvidenceStage}, Recipes: f.recipes, PreviousReport: &decision}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	f.run = stage.Run{Country: "vn", Rel: testRel, Generation: 1, Snapshot: snapshot}
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot,next_stage) VALUES ('vn',$1,'running',1,$2,'evidence') RETURNING id`, testRel, raw).Scan(&f.run.ID); err != nil {
		t.Fatal(err)
	}
	f.build = f.commit(f.run.ID, f.run.Generation, "build", batchKey, strings.Repeat("1", 64), f.recipes["build"], map[string]string{"build.json": "{}"})
	return f
}

func (f *batchFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *batchFixture) commit(runID int64, generation int, name, key, input, recipe string, files map[string]string) artifact.Result {
	f.t.Helper()
	art, err := f.env.Artifacts.Publish(f.ctx, name, input, recipe, func(dir string) error {
		for file, body := range files {
			if writeErr := os.WriteFile(filepath.Join(dir, file), []byte(body), 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.exec(`INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,$2,$3,'pending',0) ON CONFLICT DO NOTHING`, runID, name, key)
	applied, err := stage.CommitItem(f.ctx, f.db, stage.Commit{RunID: runID, Stage: name, ItemKey: key, Generation: generation, Attempt: 1, RecipeHash: recipe, Artifact: &art, State: "done"})
	if err != nil || !applied {
		f.t.Fatalf("commit %s: applied=%v err=%v", name, applied, err)
	}
	return art
}

func (f *batchFixture) commitEval(previousSHA256, marker string) string {
	f.t.Helper()
	diff := map[string]any{"reason": "no previous report"}
	if previousSHA256 != "" {
		diff = map[string]any{"previous": map[string]string{"country": "vn", "rel": previousRel, "report_sha256": previousSHA256}}
	}
	data, err := json.Marshal(map[string]any{"status": "complete", "country": "vn", "rel": testRel, "build_artifact_sha256": f.build.SHA256, "dump_sha256": strings.Repeat("d", 64), "previous_diff": diff, "marker": marker})
	if err != nil {
		f.t.Fatal(err)
	}
	f.commit(f.run.ID, f.run.Generation, "eval", batchKey, bareSHA256(data), f.recipes["eval"], map[string]string{"report.json": string(data)})
	return bareSHA256(data)
}

func (f *batchFixture) addPredecessor(frozen string) {
	f.t.Helper()
	var id int64
	if err := f.db.QueryRow(f.ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn',$1,'evaluated',1,'{"PreviousReport":{"sha256":"none"}}') RETURNING id`, previousRel).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	data, err := predecessorFile(frozen).encode()
	if err != nil {
		f.t.Fatal(err)
	}
	f.commit(id, 1, stage.EvidenceStage, "vn/"+previousRel, bareSHA256(data), f.recipes[stage.EvidenceStage], map[string]string{findingsFile: string(data)})
}

func (f *batchFixture) plan() {
	f.t.Helper()
	result, err := stage.PlanStage(f.ctx, f.env, f.stage, stage.StageInput{RunID: f.run.ID, Generation: f.run.Generation, Stage: stage.EvidenceStage}, map[string]string{stage.EvidenceStage: f.recipes[stage.EvidenceStage]})
	if err != nil {
		f.t.Fatal(err)
	}
	if len(result.Batches) != 1 || result.Batches[0] != 0 {
		f.t.Fatalf("batches = %v", result.Batches)
	}
}

func (f *batchFixture) batch(ctx context.Context, st stage.Stage, attempt int) (stage.BatchResult, error) {
	return stage.RunBatch(ctx, f.env, st, stage.BatchInput{RunID: f.run.ID, Generation: f.run.Generation, Stage: stage.EvidenceStage}, attempt, stage.BatchConfig{HeartbeatEvery: time.Minute, MaxAttempts: 1}, func(...any) {})
}

type batchItem struct {
	state      string
	generation int
	inputHash  *string
	artifactID *int64
	message    *string
}

func (f *batchFixture) item() batchItem {
	f.t.Helper()
	var item batchItem
	err := f.db.QueryRow(f.ctx, `SELECT state,generation,input_hash,artifact_id,message FROM run_item WHERE run_id=$1 AND stage='evidence' AND item_key=$2`, f.run.ID, batchKey).Scan(&item.state, &item.generation, &item.inputHash, &item.artifactID, &item.message)
	if err != nil {
		f.t.Fatal(err)
	}
	return item
}

func (f *batchFixture) evidenceArtifacts() int {
	f.t.Helper()
	var count int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM artifact WHERE stage='evidence' AND item_key=$1`, batchKey).Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	return count
}

func (f *batchFixture) findings() File {
	f.t.Helper()
	file, err := LoadFindings(f.ctx, store.New(f.db), f.env.Artifacts, "vn", testRel)
	if err != nil {
		f.t.Fatal(err)
	}
	return file
}

func (f *batchFixture) wantDone(result stage.BatchResult, err error) {
	f.t.Helper()
	if err != nil || result.Done != 1 || result.Failed != 0 || result.Gap != 0 {
		f.t.Fatalf("result=%+v err=%v", result, err)
	}
	if item := f.item(); item.state != "done" || item.artifactID == nil || item.generation != f.run.Generation {
		f.t.Fatalf("item = %+v", item)
	}
}

func TestEvidenceBatch_RefusedPlanHashIsRepaired(t *testing.T) {
	frozen := strings.Repeat("a", 64)
	f := newBatchFixture(t, stage.PreviousReport{SHA256: frozen})
	f.commitEval(frozen, "first")
	f.plan()
	refused := f.item()
	if refused.inputHash == nil {
		t.Fatal("planned item has no input hash")
	}
	result, err := f.batch(f.ctx, f.stage, 1)
	if err != nil || result.Failed != 1 || result.Done != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	failed := f.item()
	if failed.state != "failed" || failed.artifactID != nil || failed.message == nil || !strings.Contains(*failed.message, "has no run") {
		t.Fatalf("item = %+v", failed)
	}
	f.addPredecessor(frozen)
	f.exec(`UPDATE run SET generation=generation+1 WHERE id=$1`, f.run.ID)
	f.run.Generation++
	f.plan()
	if stale := f.item(); stale.state != "pending" || *stale.inputHash != *refused.inputHash {
		t.Fatalf("replanned item = %+v", stale)
	}
	result, err = f.batch(f.ctx, f.stage, 1)
	f.wantDone(result, err)
	done := f.item()
	var artifactInput string
	if err = f.db.QueryRow(f.ctx, `SELECT input_hash FROM artifact WHERE id=$1`, *done.artifactID).Scan(&artifactInput); err != nil {
		t.Fatal(err)
	}
	if *done.inputHash == *refused.inputHash || *done.inputHash != artifactInput {
		t.Fatalf("item hash %s, refused %s, artifact %s", *done.inputHash, *refused.inputHash, artifactInput)
	}
	bindings := f.findings().Bindings
	if bindings.PredecessorRel != previousRel || !strings.HasPrefix(bindings.PredecessorFindingsSHA256, hashPrefix) {
		t.Fatalf("bindings = %+v", bindings)
	}
}

func TestEvidenceBatch_RetryAfterRenameAdoptsArtifact(t *testing.T) {
	f := newBatchFixture(t, noneDecision())
	f.commitEval("", "first")
	f.plan()
	crashCtx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	if _, err := f.batch(crashCtx, crashAfterRename{Stage: f.stage, cancel: cancel}, 1); err == nil {
		t.Fatal("interrupted batch reported success")
	}
	interrupted := f.item()
	if interrupted.state != "pending" || interrupted.artifactID != nil || f.evidenceArtifacts() != 0 || f.calls.Load() != 1 {
		t.Fatalf("item=%+v artifacts=%d calls=%d", interrupted, f.evidenceArtifacts(), f.calls.Load())
	}
	if _, found, err := f.env.Artifacts.Open(stage.EvidenceStage, *interrupted.inputHash, f.recipes[stage.EvidenceStage]); err != nil || !found {
		t.Fatalf("renamed artifact found=%v err=%v", found, err)
	}
	result, err := f.batch(f.ctx, f.stage, 2)
	f.wantDone(result, err)
	if f.calls.Load() != 1 || f.evidenceArtifacts() != 1 {
		t.Fatalf("calls=%d artifacts=%d", f.calls.Load(), f.evidenceArtifacts())
	}
	if got := f.findings().Detectors["batch_check"]; !got.Complete {
		t.Fatalf("detector counts = %+v", got)
	}
}

func TestEvidenceBatch_SupersededGenerationCommitsNothing(t *testing.T) {
	f := newBatchFixture(t, noneDecision())
	f.commitEval("", "first")
	f.plan()
	f.detect = func(ctx context.Context) error {
		_, err := f.db.Exec(ctx, `UPDATE run SET generation=generation+1 WHERE id=$1`, f.run.ID)
		return err
	}
	if _, err := f.batch(f.ctx, f.stage, 1); !errors.Is(err, stage.ErrSuperseded) {
		t.Fatalf("err = %v", err)
	}
	f.detect = nil
	if item := f.item(); item.state != "pending" || item.artifactID != nil || item.generation != 0 || f.evidenceArtifacts() != 0 {
		t.Fatalf("item=%+v artifacts=%d", item, f.evidenceArtifacts())
	}
	if _, err := f.batch(f.ctx, f.stage, 2); !errors.Is(err, stage.ErrSuperseded) {
		t.Fatalf("stale batch: %v", err)
	}
	_, err := stage.PlanStage(f.ctx, f.env, f.stage, stage.StageInput{RunID: f.run.ID, Generation: f.run.Generation, Stage: stage.EvidenceStage}, nil)
	if !errors.Is(err, stage.ErrSuperseded) {
		t.Fatalf("stale plan: %v", err)
	}
	f.run.Generation++
	f.plan()
	result, err := f.batch(f.ctx, f.stage, 1)
	f.wantDone(result, err)
	if f.calls.Load() != 1 || f.evidenceArtifacts() != 1 {
		t.Fatalf("calls=%d artifacts=%d", f.calls.Load(), f.evidenceArtifacts())
	}
}

func TestEvidenceBatch_RewindThenNewEvalWritesNewEvidence(t *testing.T) {
	f := newBatchFixture(t, noneDecision())
	first := f.commitEval("", "first")
	f.exec(`INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch','vn:vbpl:1','done',0,1,1)`, f.run.ID)
	f.exec(`INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'fetch','vn:vbpl:1','source_unavailable','')`, f.run.ID)
	f.plan()
	result, err := f.batch(f.ctx, f.stage, 1)
	f.wantDone(result, err)
	old := f.item()
	var oldPath, oldSHA256 string
	if err = f.db.QueryRow(f.ctx, `SELECT path,sha256 FROM artifact WHERE id=$1`, *old.artifactID).Scan(&oldPath, &oldSHA256); err != nil {
		t.Fatal(err)
	}
	if got := f.findings().Bindings.ReportSHA256; got != hashPrefix+first {
		t.Fatalf("first report binding = %s", got)
	}
	f.exec(`UPDATE run SET state='paused',next_stage='' WHERE id=$1`, f.run.ID)
	deps := corpusrun.Deps{DB: f.db, Artifacts: f.env.Artifacts, Temporal: batchTemporal{}, Stages: f.run.Snapshot.Stages, SourceList: f.run.Snapshot.SourceList, RecipesFor: func(stage.Snapshot) map[string]string { return f.recipes }}
	if err = corpusrun.Start(f.ctx, deps, "vn", testRel, "", ""); err != nil {
		t.Fatal(err)
	}
	f.run.Generation++
	var later int
	if err = f.db.QueryRow(f.ctx, `SELECT count(*) FROM run_item WHERE run_id=$1 AND stage IN ('build','eval','evidence')`, f.run.ID).Scan(&later); err != nil || later != 0 {
		t.Fatalf("later items=%d err=%v", later, err)
	}
	f.build = f.commit(f.run.ID, f.run.Generation, "build", batchKey, strings.Repeat("1", 64), f.recipes["build"], map[string]string{"build.json": "{}"})
	second := f.commitEval("", "second")
	f.plan()
	result, err = f.batch(f.ctx, f.stage, 1)
	f.wantDone(result, err)
	current := f.item()
	if *current.artifactID == *old.artifactID || *current.inputHash == *old.inputHash || f.evidenceArtifacts() != 2 || f.calls.Load() != 2 {
		t.Fatalf("current=%+v old=%+v artifacts=%d calls=%d", current, old, f.evidenceArtifacts(), f.calls.Load())
	}
	if got := f.findings().Bindings.ReportSHA256; got != hashPrefix+second || second == first {
		t.Fatalf("second report binding = %s", got)
	}
	if sum, verifyErr := artifact.Verify(f.env.Artifacts.Abs(oldPath)); verifyErr != nil || sum != oldSHA256 {
		t.Fatalf("old artifact changed: sum=%s err=%v", sum, verifyErr)
	}
}
