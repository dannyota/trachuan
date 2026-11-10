//go:build integration

package fetch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type closeFixture struct {
	t   *testing.T
	ctx context.Context
	db  *pgxpool.Pool
	env stage.Env
	run stage.Run
}

func newCloseFixture(t *testing.T, version int, staged bool) *closeFixture {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	data, err := json.Marshal(source.List{Country: "vn", VBPL: source.VBPLSources{SelectionVersion: version}, RelationCodes: map[string]source.RelationCode{"A": {Kind: "cites"}}})
	if err != nil {
		t.Fatal(err)
	}
	r := stage.Run{Country: "vn", Rel: "20990103", Snapshot: stage.Snapshot{SourceList: data}}
	if staged {
		r.Snapshot.Stages = []string{"discover", "expand", "fetch"}
	}
	snapshot, err := json.Marshal(r.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,next_stage,snapshot) VALUES ('vn',$1,'running','fetch',$2) RETURNING id`, r.Rel, snapshot).Scan(&r.ID); err != nil {
		t.Fatal(err)
	}
	return &closeFixture{t, ctx, db, stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}, r}
}
func (f *closeFixture) phase(p membership.Phase, entries ...membership.Entry) {
	f.t.Helper()
	if err := membership.PublishPhase(f.ctx, store.New(f.db), f.run.ID, p, entries); err != nil {
		f.t.Fatal(err)
	}
}
func (f *closeFixture) bundle(target string) {
	f.t.Helper()
	detail := `{"data":{"references":[]}}`
	if target != "" {
		detail = `{"data":{"references":[{"referenceType":"A","targetDocument":{"id":"` + target + `"}}]}}`
	}
	art, err := f.env.Artifacts.PublishContent(f.ctx, "fetch", stage.RecipeHash("test", "fetch"), func(dir string) error {
		if e := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(detail), 0600); e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dir, "diagram.json"), []byte(`{"data":{}}`), 0600)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	var aid int64
	if err = f.db.QueryRow(f.ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, stage.RecipeHash("test", "fetch"), art.Path, art.SHA256).Scan(&aid); err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, f.run.ID, aid); err != nil {
		f.t.Fatal(err)
	}
}
func (f *closeFixture) finalize() (stage.Finalization, error) {
	f.t.Helper()
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	out, err := New(nil).Finalize(f.ctx, f.env, tx, f.run)
	if err != nil {
		return out, err
	}
	if err = tx.Commit(f.ctx); err != nil {
		return out, err
	}
	return out, nil
}
func closeSeed(id string) membership.Entry {
	return membership.Entry{DocumentKey: "vn:vbpl:" + id, Source: "vbpl", SourceID: id, Class: "primary", Disposition: "active"}
}
func TestFinalize_NoDriftPublishesFinalIdempotently(t *testing.T) {
	f := newCloseFixture(t, 1, true)
	f.phase(membership.Expanded, closeSeed("1"), closeSeed("2"))
	f.bundle("2")
	for i := 0; i < 2; i++ {
		out, err := f.finalize()
		if err != nil || out.Code != "" {
			t.Fatalf("result=%#v err=%v", out, err)
		}
	}
	s, err := membership.ReadPhase(f.ctx, store.New(f.db), f.run.ID, membership.Final)
	if err != nil || len(s.Active) != 2 {
		t.Fatalf("snapshot=%#v err=%v", s, err)
	}
}
func TestFinalize_LegacyFetchCloseUnchanged(t *testing.T) {
	f := newCloseFixture(t, 0, false)
	if _, err := f.finalize(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM run_scope WHERE run_id=$1`, f.run.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("scope count=%d err=%v", n, err)
	}
}
func TestFinalize_WithoutExpandPublishesSeeds(t *testing.T) {
	f := newCloseFixture(t, 1, false)
	f.phase(membership.Seeds, closeSeed("1"))
	if _, err := f.finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := membership.ReadPhase(f.ctx, store.New(f.db), f.run.ID, membership.Final); err != nil {
		t.Fatal(err)
	}
}
func TestDrift_GapCannotBeAccepted(t *testing.T) {
	f := newCloseFixture(t, 1, true)
	f.phase(membership.Expanded, closeSeed("1"))
	f.bundle("2")
	result, err := stage.CloseStage(f.ctx, f.env, New(nil), stage.StageInput{RunID: f.run.ID, Stage: "fetch"}, "extract")
	if err != nil || !result.Failed || !strings.Contains(result.Reason, "vn:vbpl:2") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	_, err = run.AcceptGaps(f.ctx, run.Deps{DB: f.db}, "vn", f.run.Rel, "any", "accept")
	if err == nil || !strings.Contains(err.Error(), "scope_changed") {
		t.Fatalf("accept error=%v", err)
	}
	if _, err = membership.ReadPhase(f.ctx, store.New(f.db), f.run.ID, membership.Final); err == nil {
		t.Fatal("published final membership after drift")
	}
}

func TestFinalize_RetainedRemovalDriftInTransaction(t *testing.T) {
	f := newCloseFixture(t, 1, true)
	removed := closeSeed("2")
	removed.Disposition = "scope_removed"
	f.phase(membership.Expanded, closeSeed("1"), removed)
	f.bundle("2")
	got, err := f.finalize()
	if err != nil || got.Code != "scope_changed" || got.ItemKey != "vn:vbpl:1" || len(got.EvidenceRefs) != 1 || !strings.Contains(got.EvidenceRefs[0], "reason=retained_removal") {
		t.Fatalf("result=%#v err=%v", got, err)
	}
	if _, err = membership.ReadPhase(f.ctx, store.New(f.db), f.run.ID, membership.Final); err == nil {
		t.Fatal("published final after drift")
	}
}
func TestFinalize_RejectsChangedFetchInputs(t *testing.T) {
	f := newCloseFixture(t, 1, true)
	f.phase(membership.Expanded, closeSeed("1"))
	f.bundle("")
	if _, err := f.finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx, `UPDATE run_item SET state='gap' WHERE run_id=$1 AND stage='fetch'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.finalize(); err == nil {
		t.Fatal("accepted changed fetch input")
	}
}
func TestFinalize_RequiresExpandedPhaseWhenStaged(t *testing.T) {
	f := newCloseFixture(t, 1, true)
	f.phase(membership.Seeds, closeSeed("1"))
	if _, err := f.finalize(); err == nil {
		t.Fatal("accepted unfrozen expanded membership")
	}
}

func TestFinalize_DeduplicatesReferencesAndSkipsAttachmentHash(t *testing.T) {
	f := newCloseFixture(t, 1, true)
	f.phase(membership.Expanded, closeSeed("1"))
	detail := []byte(`{"data":{"references":[{"referenceType":"A","targetDocument":{"id":"2"}}]}}`)
	diagram := []byte(`{"data":{"documentNamesByType":{"A":[{"id":"2"}]}}}`)
	art, err := f.env.Artifacts.PublishContent(f.ctx, "fetch", stage.RecipeHash("test", "both"), func(dir string) error {
		for _, item := range []struct {
			name string
			body []byte
		}{{"detail.json", detail}, {"diagram.json", diagram}, {"attachment", []byte("original")}} {
			if e := os.WriteFile(filepath.Join(dir, item.name), item.body, 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var aid int64
	if err = f.db.QueryRow(f.ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, art.InputHash, stage.RecipeHash("test", "both"), art.Path, art.SHA256).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, f.run.ID, aid); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(f.env.Artifacts.Abs(art.Path), "attachment"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := f.finalize()
	if err != nil || got.Code != "scope_changed" || len(got.EvidenceRefs) != 1 || !strings.HasPrefix(got.Reason, "1 scope drift") {
		t.Fatalf("result=%#v err=%v", got, err)
	}
}
