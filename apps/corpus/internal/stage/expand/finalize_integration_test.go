//go:build integration

package expand

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	t   *testing.T
	ctx context.Context
	db  *pgxpool.Pool
	env stage.Env
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return &fixture{t, ctx, db, stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}}
}
func (f *fixture) run(rel string) stage.Run {
	f.t.Helper()
	data, err := json.Marshal(source.List{Country: "vn", VBPL: source.VBPLSources{SelectionVersion: 1}})
	if err != nil {
		f.t.Fatal(err)
	}
	r := stage.Run{Country: "vn", Rel: rel, Snapshot: stage.Snapshot{SourceList: data}}
	snap, err := json.Marshal(r.Snapshot)
	if err != nil {
		f.t.Fatal(err)
	}
	if err = f.db.QueryRow(f.ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn',$1,'paused',$2) RETURNING id`, rel, snap).Scan(&r.ID); err != nil {
		f.t.Fatal(err)
	}
	return r
}
func (f *fixture) phase(r stage.Run, p membership.Phase, entries ...membership.Entry) {
	f.t.Helper()
	if err := membership.PublishPhase(f.ctx, store.New(f.db), r.ID, p, entries); err != nil {
		f.t.Fatal(err)
	}
	if p == membership.Seeds {
		_, err := f.db.Exec(f.ctx, `UPDATE run_scope SET predecessor_run_id = prior.run_id, predecessor_phase = 'final', predecessor_snapshot_hash = prior.final_hash FROM (SELECT s.run_id,s.final_hash FROM run_scope s JOIN run old ON old.id=s.run_id WHERE old.country=$2 AND old.rel < $3 AND s.final_snapshot IS NOT NULL ORDER BY old.rel DESC LIMIT 1) prior WHERE run_scope.run_id=$1`, r.ID, r.Country, r.Rel)
		if err != nil {
			f.t.Fatal(err)
		}
	}
}
func (f *fixture) item(r stage.Run, key, state string, obs *Observation) {
	f.t.Helper()
	var aid *int64
	if obs != nil {
		art, err := f.env.Artifacts.PublishContent(f.ctx, "expand", stage.RecipeHash("test", r.Rel, key), func(dir string) error {
			b, e := json.Marshal(obs)
			if e != nil {
				return e
			}
			return os.WriteFile(filepath.Join(dir, "observation.json"), b, 0600)
		})
		if err != nil {
			f.t.Fatal(err)
		}
		var id int64
		if err = f.db.QueryRow(f.ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('expand',$1,$2,$3,$4,$5) RETURNING id`, key, art.InputHash, stage.RecipeHash("test", r.Rel, key), art.Path, art.SHA256).Scan(&id); err != nil {
			f.t.Fatal(err)
		}
		aid = &id
	}
	if _, err := f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'expand',$2,$3,0,$4)`, r.ID, key, state, aid); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) central(r stage.Run) {
	f.t.Helper()
	if _, err := f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'discover','vbpl:central','done',0)`, r.ID); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) close(r stage.Run) error {
	f.t.Helper()
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = New(nil).Finalize(f.ctx, f.env, tx, r)
	if err != nil {
		return err
	}
	return tx.Commit(f.ctx)
}
func TestExpand_FinalizeRetainedAndIdempotent(t *testing.T) {
	f := newFixture(t)
	old := f.run("20990101")
	f.phase(old, membership.Final, testSeed("1"), testContext("2", "1"))
	r := f.run("20990102")
	f.phase(r, membership.Seeds, testSeed("1"))
	f.item(r, "vn:vbpl:1", "gap", nil)
	if err := f.close(r); err != nil {
		t.Fatal(err)
	}
	if err := f.close(r); err != nil {
		t.Fatal(err)
	}
	s, err := membership.ReadPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded)
	if err != nil || len(s.Active) != 2 {
		t.Fatalf("snapshot = %#v, error = %v", s, err)
	}
	records, err := membership.ReadEvidence(f.ctx, store.New(f.db), r.ID)
	if err != nil || len(records) != 1 || records[0].State != "retained" {
		t.Fatalf("records = %#v, error = %v", records, err)
	}
}
func TestExpand_FinalizeDepartedSeed(t *testing.T) {
	f := newFixture(t)
	old := f.run("20990101")
	f.phase(old, membership.Final, testSeed("1"), testContext("2", "1"))
	r := f.run("20990102")
	f.phase(r, membership.Seeds)
	f.central(r)
	if err := f.close(r); err != nil {
		t.Fatal(err)
	}
	s, err := membership.ReadPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded)
	if err != nil || len(s.RetainedRemovals) != 1 || s.RetainedRemovals[0].Disposition != "scope_removed" {
		t.Fatalf("snapshot = %#v, error = %v", s, err)
	}
}

func TestExpand_FinalizeCurrentTargetAndFrozenEvidence(t *testing.T) {
	f := newFixture(t)
	r := f.run("20990103")
	removed := testSeed("2")
	removed.Disposition = "withdrawn"
	f.phase(r, membership.Seeds, testSeed("1"), removed)
	obs := Observation{Version: 1, SeedKey: "vn:vbpl:1", Targets: []Target{{TargetKey: "vn:vbpl:2", RawCode: "A", Kind: "cites", Method: "vbpl_references", Response: "detail.json", ResponseSHA256: "response"}}}
	f.item(r, "vn:vbpl:1", "done", &obs)
	if err := f.close(r); err != nil {
		t.Fatal(err)
	}
	s, err := membership.ReadPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded)
	if err != nil || len(s.Active) != 2 || s.Active[1].Class != "relation_context" {
		t.Fatalf("snapshot=%#v error=%v", s, err)
	}
	rows, err := store.New(f.db).ListSeedObservations(f.ctx, r.ID)
	if err != nil || len(rows) != 1 || rows[0].Completeness != "complete" || string(rows[0].ObservedTargets) != `["vn:vbpl:2"]` {
		t.Fatalf("observations=%#v error=%v", rows, err)
	}
	if err = f.close(r); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run_membership_evidence SET support_state='removed' WHERE run_id=$1 AND rule_kind='relation'`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.close(r); err == nil {
		t.Fatal("accepted changed frozen evidence")
	}
}
func TestExpand_FrozenEquivalentIncompleteState(t *testing.T) {
	f := newFixture(t)
	r := f.run("20990103")
	f.phase(r, membership.Seeds, testSeed("1"))
	f.item(r, "vn:vbpl:1", "gap", nil)
	if err := f.close(r); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx, `UPDATE run_item SET state='done' WHERE run_id=$1 AND stage='expand'`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.close(r); err != nil {
		t.Fatalf("same frozen outputs: %v", err)
	}
	if _, err := f.db.Exec(f.ctx, `UPDATE run_seed_observation SET observed_targets='["vn:vbpl:9"]' WHERE run_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.close(r); err == nil {
		t.Fatal("accepted changed frozen observation")
	}
}
func TestExpand_RejectsMissingOrUnexpectedItems(t *testing.T) {
	for _, tc := range []struct{ name, key string }{{"missing", ""}, {"unexpected", "vn:vbpl:2"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			r := f.run("20990103")
			f.phase(r, membership.Seeds, testSeed("1"))
			if tc.key != "" {
				f.item(r, tc.key, "gap", nil)
			}
			if err := f.close(r); err == nil {
				t.Fatal("accepted mismatched items")
			}
		})
	}
}
func TestExpand_RejectsInvalidObservation(t *testing.T) {
	for _, obs := range []Observation{{Version: 2, SeedKey: "vn:vbpl:1"}, {Version: 1, SeedKey: "vn:vbpl:2"}} {
		f := newFixture(t)
		r := f.run("20990103")
		f.phase(r, membership.Seeds, testSeed("1"))
		f.item(r, "vn:vbpl:1", "done", &obs)
		if err := f.close(r); err == nil {
			t.Fatal("accepted invalid observation")
		}
	}
}
func TestExpand_DepartedSupportNeedsDiscoveryItem(t *testing.T) {
	f := newFixture(t)
	old := f.run("20990101")
	f.phase(old, membership.Final, testContext("2", "1"))
	r := f.run("20990102")
	f.phase(r, membership.Seeds)
	if err := f.close(r); err == nil {
		t.Fatal("accepted departed support without discovery item")
	}
}
func TestExpand_PlanRequiresFrozenSeeds(t *testing.T) {
	f := newFixture(t)
	r := f.run("20990103")
	if _, err := New(nil).Plan(f.ctx, f.env, r); err == nil {
		t.Fatal("accepted unfrozen seeds")
	}
	context := testContext("2", "1")
	f.phase(r, membership.Seeds, testSeed("1"), context)
	items, err := New(nil).Plan(f.ctx, f.env, r)
	if err != nil || len(items) != 1 || items[0].Key != "vn:vbpl:1" {
		t.Fatalf("items=%#v error=%v", items, err)
	}
}

func TestExpand_RefreshedRuleAndRemovalAcrossRuns(t *testing.T) {
	f := newFixture(t)
	for _, rel := range []string{"20990101", "20990102", "20990103", "20990104", "20990105"} {
		r := f.run(rel)
		f.phase(r, membership.Seeds, testSeed("1"))
		obs := Observation{Version: 1, SeedKey: "vn:vbpl:1"}
		state := "current"
		disposition := "active"
		if rel < "20990104" {
			obs.Targets = []Target{{TargetKey: "vn:vbpl:2", RawCode: "A", Kind: "cites", Method: "vbpl_references", Response: "detail.json", ResponseSHA256: rel}}
		} else {
			state = "removed"
			disposition = "withdrawn"
		}
		f.item(r, "vn:vbpl:1", "done", &obs)
		if err := f.close(r); err != nil {
			t.Fatal(err)
		}
		s, err := membership.ReadPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded)
		if err != nil || len(s.Entries) != 2 || s.Entries[1].Disposition != disposition || len(s.Entries[1].Supports) != 1 {
			t.Fatalf("%s snapshot=%#v err=%v", rel, s, err)
		}
		records, err := membership.ReadEvidence(f.ctx, store.New(f.db), r.ID)
		f.phase(r, membership.Final, s.Entries...)
		if err != nil || len(records) != 1 || records[0].State != state {
			t.Fatalf("%s records=%#v err=%v", rel, records, err)
		}
	}
}
