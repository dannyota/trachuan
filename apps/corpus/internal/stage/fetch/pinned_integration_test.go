//go:build integration

package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/stage/expand"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pinPipeline struct {
	env           stage.Env
	run           stage.Run
	pin           source.Pin
	expand, fetch artifact.Result
}

func newPinPipeline(t *testing.T) *pinPipeline {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	f := &pinPipeline{env: stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}, pin: source.Pin{Key: "vn:vbpl:pin", Number: "71/2099/QH15", Rel: "20990101_1", Since: "2099-01-02"}}
	var producer int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES('vn',$1,'paused','{}') RETURNING id`, f.pin.Rel).Scan(&producer); err != nil {
		t.Fatal(err)
	}
	detail := []byte(`{"data":{"id":"pin","docNum":"71/2099/QH15","references":[{"referenceType":"1","targetDocument":{"id":"context"}}]}}`)
	for _, name := range []string{"expand", "fetch"} {
		files := map[string][]byte{"detail.json": detail, "diagram.json": []byte(`{"data":{}}`)}
		if name == "expand" {
			files["observation.json"] = []byte(`{}`)
		} else {
			files["tree.json"] = []byte(`{"data":[]}`)
			files["files.json"] = []byte(`[]`)
			files["report.json"] = []byte(`{"missing":["missing.pdf"],"too_large":["large.pdf"]}`)
		}
		art, publishErr := f.env.Artifacts.PublishContent(ctx, name, strings.Repeat("a", 64), func(dir string) error {
			for path, data := range files {
				if writeErr := os.WriteFile(filepath.Join(dir, path), data, 0600); writeErr != nil {
					return writeErr
				}
			}
			return nil
		})
		if publishErr != nil {
			t.Fatal(publishErr)
		}
		registerPinItem(t, f.env, producer, name, f.pin.Key, art, strings.Repeat("a", 64))
		if name == "expand" {
			f.expand = art
			f.pin.Expand = art.SHA256
		} else {
			f.fetch = art
			f.pin.Fetch = art.SHA256
		}
	}
	f.run = stage.Run{Country: "vn", Rel: "20990102_1", Generation: 1}
	f.setPins(t, []source.Pin{f.pin})
	return f
}

func (f *pinPipeline) setPins(t *testing.T, pins []source.Pin) {
	t.Helper()
	ctx := context.Background()
	raw, err := json.Marshal(source.List{Country: "vn", VBPL: source.VBPLSources{SelectionVersion: 1, Pinned: pins}, RelationCodes: map[string]source.RelationCode{"1": {Kind: "repeals"}}})
	if err != nil {
		t.Fatal(err)
	}
	f.run.Snapshot = stage.Snapshot{SourceList: raw, SourceListHash: digest(raw), Recipes: map[string]string{}}
	f.run.Snapshot.Recipes["discover"] = discover.New(nil).Recipe(f.run.Snapshot)
	snapshot, err := json.Marshal(f.run.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if f.run.ID == 0 {
		err = f.env.DB.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,next_stage,snapshot) VALUES('vn',$1,'running',1,'discover',$2) RETURNING id`, f.run.Rel, snapshot).Scan(&f.run.ID)
	} else {
		_, err = f.env.DB.Exec(ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, f.run.ID, snapshot)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func registerPinItem(t *testing.T, env stage.Env, runID int64, name, key string, art artifact.Result, recipe string) {
	t.Helper()
	ctx := context.Background()
	var id int64
	err := env.DB.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(stage,input_hash,recipe_hash) DO UPDATE SET sha256=EXCLUDED.sha256 RETURNING id`, name, key, art.InputHash, recipe, art.Path, art.SHA256).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt,artifact_id,input_hash,recipe_hash) VALUES($1,$2,$3,'done',0,1,1,$4,$5,$6) ON CONFLICT(run_id,stage,item_key) DO UPDATE SET state='done',artifact_id=EXCLUDED.artifact_id,input_hash=EXCLUDED.input_hash,recipe_hash=EXCLUDED.recipe_hash`, runID, name, key, id, art.InputHash, recipe)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPinInvalid_DurableBeforeListing(t *testing.T) {
	for _, name := range []string{"missing artifact", "altered file", "wrong rel", "wrong stage", "wrong key", "number mismatch", "duplicate key", "duplicate normalized number"} {
		t.Run(name, func(t *testing.T) {
			f := newPinPipeline(t)
			pins := []source.Pin{f.pin}
			ctx := context.Background()
			switch name {
			case "missing artifact":
				if err := os.RemoveAll(f.env.Artifacts.Abs(f.fetch.Path)); err != nil {
					t.Fatal(err)
				}
			case "altered file":
				if err := os.WriteFile(filepath.Join(f.env.Artifacts.Abs(f.fetch.Path), "tree.json"), []byte("altered"), 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong rel":
				pins[0].Rel = "20990101_2"
			case "wrong stage":
				if _, err := f.env.DB.Exec(ctx, `UPDATE run_item SET stage='relate' WHERE stage='expand'`); err != nil {
					t.Fatal(err)
				}
			case "wrong key":
				pins[0].Key = "vn:vbpl:other"
			case "number mismatch":
				pins[0].Number = "72/2099/QH15"
			case "duplicate key":
				pins = append(pins, f.pin)
			case "duplicate normalized number":
				other := f.pin
				other.Key = "vn:vbpl:other"
				other.Number = " 71 / 2099 / qh15 "
				pins = append(pins, other)
			}
			f.setPins(t, pins)
			in := stage.StageInput{RunID: f.run.ID, Generation: 1, Stage: "discover"}
			for range 2 {
				result, err := stage.PlanStage(ctx, f.env, discover.New(nil), in, nil)
				if err != nil || len(result.Batches) != 0 {
					t.Fatalf("plan=%+v err=%v", result, err)
				}
			}
			var code, state string
			var items, requests int
			err := f.env.DB.QueryRow(ctx, `SELECT r.state,f.code,(SELECT count(*) FROM run_item WHERE run_id=r.id),(SELECT count(*) FROM run_source_request WHERE run_id=r.id) FROM run r JOIN run_failure f ON f.run_id=r.id WHERE r.id=$1`, f.run.ID).Scan(&state, &code, &items, &requests)
			if err != nil || code != "pin_invalid" || state != "failed" || items != 0 || requests != 0 {
				t.Fatalf("state=%s code=%s items=%d requests=%d err=%v", state, code, items, requests, err)
			}
			closed, err := stage.CloseStage(ctx, f.env, discover.New(nil), in, "expand")
			if err != nil || !closed.Failed || closed.NextStage != "" {
				t.Fatalf("close=%+v err=%v", closed, err)
			}
		})
	}
}

func TestPinned_AbsentMembershipContextAndBundle(t *testing.T) {
	f := newPinPipeline(t)
	ctx := context.Background()
	in := stage.StageInput{RunID: f.run.ID, Generation: 1, Stage: "discover"}
	if _, err := stage.PlanStage(ctx, f.env, discover.New(nil), in, nil); err != nil {
		t.Fatal(err)
	}
	art, err := f.env.Artifacts.PublishContent(ctx, "discover", discover.New(nil).Recipe(f.run.Snapshot), func(dir string) error {
		for name, data := range map[string]string{"report.json": `{"version":2,"stable":true}`, "catalog.json": `{"complete":true,"ids":[]}`, "selected.json": `{"items":[]}`} {
			if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registerPinItem(t, f.env, f.run.ID, "discover", "vbpl:central", art, discover.New(nil).Recipe(f.run.Snapshot))
	closePinPhase(t, f, discover.New(nil))
	expanded := expand.New(nil)
	processPinItem(t, f, expanded)
	closePinPhase(t, f, expanded)
	phase, err := membership.ReadPhase(ctx, store.New(f.env.DB), f.run.ID, membership.Expanded)
	if err != nil {
		t.Fatal(err)
	}
	if len(phase.Active) != 2 {
		t.Fatalf("active=%+v", phase.Active)
	}
	for _, entry := range phase.Active {
		if entry.DocumentKey == "vn:vbpl:context" && (entry.Class != "relation_context" || len(entry.Supports) == 0 || entry.Supports[0].RuleKind != "relation") {
			t.Fatalf("context=%+v", entry)
		}
	}
	fetched := processPinItem(t, f, New(nil))
	if fetched.Artifact.SHA256 != f.fetch.SHA256 {
		t.Fatal("pinned fetch changed")
	}
	for _, name := range []string{"detail.json", "diagram.json", "tree.json", "files.json", "report.json", "MANIFEST.sha256"} {
		before, readErr := os.ReadFile(filepath.Join(f.env.Artifacts.Abs(f.fetch.Path), name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		after, readErr := os.ReadFile(filepath.Join(f.env.Artifacts.Abs(fetched.Artifact.Path), name))
		if readErr != nil || !bytes.Equal(before, after) {
			t.Fatalf("bundle %s changed: %v", name, readErr)
		}
	}
	var baseline, requests, observations, gaps int
	err = f.env.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM fetch_baseline WHERE document_key=$1),(SELECT count(*) FROM run_source_request WHERE run_id=$2),(SELECT count(*) FROM run_fetch_observation WHERE run_id=$2 AND acquisition='pinned'),(SELECT count(*) FROM gap WHERE run_id=$2 AND kind IN ('source_withdrawn_retained','missing_attachment','file_too_large'))`, f.pin.Key, f.run.ID).Scan(&baseline, &requests, &observations, &gaps)
	if err != nil || baseline != 0 || requests != 0 || observations != 3 || gaps != 3 {
		t.Fatalf("baseline=%d requests=%d observations=%d gaps=%d err=%v", baseline, requests, observations, gaps, err)
	}
}

func TestPinConflict_DurableBeforeExpand(t *testing.T) {
	f := newPinPipeline(t)
	ctx := context.Background()
	processor := discover.New(nil)
	in := stage.StageInput{RunID: f.run.ID, Generation: 1, Stage: "discover"}
	if _, err := stage.PlanStage(ctx, f.env, processor, in, nil); err != nil {
		t.Fatal(err)
	}
	art, err := f.env.Artifacts.PublishContent(ctx, "discover", processor.Recipe(f.run.Snapshot), func(dir string) error {
		for name, data := range map[string]string{"report.json": `{"version":2,"stable":true}`, "catalog.json": `{"complete":true,"ids":["duplicate"]}`, "selected.json": `{"items":[]}`} {
			if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registerPinItem(t, f.env, f.run.ID, "discover", "vbpl:central", art, processor.Recipe(f.run.Snapshot))
	payload := []byte(`{"id":"duplicate","number":"71/2099/QH15","rules":[{"kind":"term","value":"law"}],"page":{"pass":2,"page":1,"sha256":"response","path":"page.json"}}`)
	if _, err = f.env.DB.Exec(ctx, `INSERT INTO listing(run_id,query_key,item_key,payload) VALUES($1,'listing','vn:vbpl:duplicate',$2)`, f.run.ID, payload); err != nil {
		t.Fatal(err)
	}
	result, err := stage.CloseStage(ctx, f.env, processor, in, "expand")
	if err != nil || !result.Failed || result.NextStage != "" {
		t.Fatalf("close=%+v err=%v", result, err)
	}
	var code string
	if err = f.env.DB.QueryRow(ctx, `SELECT code FROM run_failure WHERE run_id=$1`, f.run.ID).Scan(&code); err != nil || code != "pin_conflict" {
		t.Fatalf("code=%s err=%v", code, err)
	}
}

func processPinItem(t *testing.T, f *pinPipeline, s stage.Stage) stage.Output {
	t.Helper()
	ctx := context.Background()
	item := stage.Item{Key: f.pin.Key}
	if _, err := f.env.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,$2,$3,'pending',0)`, f.run.ID, s.Name(), item.Key); err != nil {
		t.Fatal(err)
	}
	out, err := s.Process(ctx, f.env, f.run, item)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.Rows(ctx, f.env, *out.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Resolve(ctx, f.env, f.run, item, *out.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := stage.CommitItem(ctx, f.env.DB, stage.Commit{RunID: f.run.ID, Stage: s.Name(), ItemKey: item.Key, Generation: 1, Attempt: 1, RecipeHash: s.Recipe(f.run.Snapshot), Artifact: out.Artifact, ArtifactRows: rows, RunRows: res.RunRows, Observations: out.Observations, Gaps: res.Gaps, State: out.State, Message: out.Message, Sources: out.Sources, SourceRead: true})
	if err != nil || !applied {
		t.Fatalf("commit=%v err=%v", applied, err)
	}
	return out
}

func closePinPhase(t *testing.T, f *pinPipeline, s stage.Finalizer) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.env.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := s.Finalize(ctx, f.env, tx, f.run)
	if err != nil || result.Code != "" {
		t.Fatalf("finalize=%+v err=%v", result, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
