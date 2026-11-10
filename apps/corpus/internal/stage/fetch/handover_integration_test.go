//go:build integration

package fetch

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type handOverFixture struct {
	*closeFixture
	stage         *Stage
	secondary     membership.Entry
	candidate     artifact.Result
	primaryHashes map[string]string
}

type extraPrimary struct{ id, number, date string }

func newHandOverFixture(t *testing.T, discoveryDate, fetchDate, secondaryState, vbplState string, extra ...extraPrimary) *handOverFixture {
	t.Helper()
	f := &handOverFixture{closeFixture: newCloseFixture(t, 1, false), stage: New(nil).WithChinhphu(chinhphu.New()), primaryHashes: map[string]string{}}
	const query = "chinhphu:keyword:law"
	detail := []byte("recorded secondary detail")
	probe := []byte("recorded probe response")
	state := "match"
	if discoveryDate != "2026-01-01" {
		state = "different_date"
	}
	candidate := map[string]any{"candidate": map[string]any{"id": "5", "state": "seed_primary", "probe_state": "complete", "number": "42/2026/QD", "issuer_form": "Government", "date": "2026-01-01", "primary_id": "1", "detail_path": "detail/p5-1", "detail_hash": fmt.Sprintf("%x", sha256.Sum256(detail)), "probe_passes": []any{map[string]any{"version": 1, "items": []any{}, "evidence": map[string]any{"completeness": "complete", "pages_fetched": []any{map[string]any{"path": "probe-5/p1-1", "sha256": fmt.Sprintf("%x", sha256.Sum256(probe))}}}}}, "comparisons": []any{map[string]any{"id": "1", "state": state, "item": map[string]any{"source_id": "1", "raw_number": "42/2026/QD", "issuer": "Government", "issue_date": discoveryDate}}}}}
	body, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	art, err := f.env.Artifacts.PublishContent(f.ctx, "discover", stage.RecipeHash("test", "candidate"), func(dir string) error {
		for _, dirName := range []string{"detail", "probe-5"} {
			if e := os.Mkdir(filepath.Join(dir, dirName), 0700); e != nil {
				return e
			}
		}
		for name, data := range map[string][]byte{"candidate-5.json": body, "detail/p5-1": detail, "probe-5/p1-1": probe} {
			if e := os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.candidate = art
	f.insertArtifactItem("discover", query, "done", &art)
	support := membership.SupportTuple{RuleKind: "secondary_probe", RuleValue: query + "|5", Evidence: membership.EvidenceRef{ArtifactHash: art.SHA256, Path: "candidate-5.json", ResponseSHA256: fmt.Sprintf("%x", sha256.Sum256(body))}}
	f.secondary = membership.Entry{DocumentKey: "vn:chinhphu:5", Source: "chinhphu", SourceID: "5", Class: "secondary", Disposition: "active", Supports: []membership.SupportTuple{support}}
	entries := []membership.Entry{closeSeed("1"), f.secondary}
	for _, x := range extra {
		entries = append(entries, closeSeed(x.id))
	}
	f.phase(membership.Seeds, entries...)
	if secondaryState == "done" {
		f.secondaryBundle()
	} else {
		f.insertArtifactItem("fetch", f.secondary.DocumentKey, "gap", nil)
	}
	if vbplState == "done" {
		raw := []byte(fmt.Sprintf(`{"data":{"id":"1","docNum":"42/2026/QD","agencyName":"Government","issueDate":%q}}`, fetchDate))
		a, e := f.env.Artifacts.PublishContent(f.ctx, "fetch", stage.RecipeHash("test", "vbpl"), func(dir string) error { return os.WriteFile(filepath.Join(dir, "detail.json"), raw, 0600) })
		if e != nil {
			t.Fatal(e)
		}
		f.insertArtifactItem("fetch", "vn:vbpl:1", "done", &a)
		f.primaryHashes["1"] = a.SHA256
	} else {
		f.insertArtifactItem("fetch", "vn:vbpl:1", "gap", nil)
		if _, err = f.db.Exec(f.ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'fetch','vn:vbpl:1','source_unavailable','unavailable')`, f.run.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, x := range extra {
		raw := []byte(fmt.Sprintf(`{"data":{"id":%q,"docNum":%q,"agencyName":"Government","issueDate":%q}}`, x.id, x.number, x.date))
		a, e := f.env.Artifacts.PublishContent(f.ctx, "fetch", stage.RecipeHash("test", "vbpl", x.id), func(dir string) error { return os.WriteFile(filepath.Join(dir, "detail.json"), raw, 0600) })
		if e != nil {
			t.Fatal(e)
		}
		f.insertArtifactItem("fetch", "vn:vbpl:"+x.id, "done", &a)
		f.primaryHashes[x.id] = a.SHA256
	}
	switch secondaryState {
	case "skip":
		_, e := f.db.Exec(f.ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'fetch','vn:chinhphu:5','source_replaced','vn:chinhphu:5 -> vn:vbpl:1')`, f.run.ID)
		if e != nil {
			t.Fatal(e)
		}
	case "gap":
		_, e := f.db.Exec(f.ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'fetch','vn:chinhphu:5','not_found','missing')`, f.run.ID)
		if e != nil {
			t.Fatal(e)
		}
	}
	return f
}
func (f *handOverFixture) insertArtifactItem(stageName, key, state string, art *artifact.Result) {
	f.t.Helper()
	var id *int64
	if art != nil {
		var value int64
		err := f.db.QueryRow(f.ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, stageName, key, art.InputHash, stage.RecipeHash("test", stageName, key), art.Path, art.SHA256).Scan(&value)
		if err != nil {
			f.t.Fatal(err)
		}
		id = &value
	}
	if _, err := f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,$2,$3,$4,0,$5)`, f.run.ID, stageName, key, state, id); err != nil {
		f.t.Fatal(err)
	}
}
func (f *handOverFixture) secondaryBundle() {
	f.t.Helper()
	html := []byte(`<div id="ctrl_190596_91_Content"><table><tr><td>Số ký hiệu</td><td>42/2026/QD</td></tr><tr><td>Cơ quan ban hành</td><td>Government</td></tr><tr><td>Ngày ban hành</td><td>01-01-2026</td></tr></table></div>`)
	d, err := f.stage.chinhphu.ParseDetail(html, "https://vanban.chinhphu.vn/?pageid=27160&docid=5")
	if err != nil {
		f.t.Fatal(err)
	}
	art, err := f.env.Artifacts.PublishContent(f.ctx, "fetch", stage.RecipeHash("test", "secondary"), func(dir string) error {
		var observations []stage.FetchObservation
		return f.stage.writeChinhphu(f.ctx, dir, "5", d, html, &observations)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.insertArtifactItem("fetch", f.secondary.DocumentKey, "done", &art)
}
func (f *handOverFixture) close() {
	f.t.Helper()
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	if _, err = f.stage.Finalize(f.ctx, f.env, tx, f.run); err != nil {
		f.t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}
func (f *handOverFixture) check(kind, disposition string) {
	f.t.Helper()
	phase, err := membership.ReadPhase(f.ctx, store.New(f.db), f.run.ID, membership.Final)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, entry := range phase.Entries {
		if entry.DocumentKey == f.secondary.DocumentKey && entry.Disposition != disposition {
			f.t.Fatalf("secondary=%+v", entry)
		}
	}
	gaps, err := store.New(f.db).ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	count := 0
	for _, gap := range gaps {
		if gap.ItemKey == f.secondary.DocumentKey && gap.Kind == kind {
			count++
			if gap.EvidenceRef == nil || !strings.Contains(*gap.EvidenceRef, f.candidate.SHA256+"/candidate-5.json") {
				f.t.Fatalf("gap=%+v", gap)
			}
		}
	}
	if kind != "" && count != 1 {
		f.t.Fatalf("gaps=%+v", gaps)
	}
}
func TestHandOver_SkippedDownloadUsesDiscoveryRecord(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "skip", "done")
	f.close()
	f.check("source_replaced", "source_replaced")
}
func TestHandOver_SkipConflictBecomesAmbiguous(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-02", "skip", "done")
	f.close()
	f.check("source_identity_ambiguous", "active")
	gaps, err := store.New(f.db).ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, gap := range gaps {
		if gap.ItemKey == f.secondary.DocumentKey && gap.Kind == "source_replaced" {
			t.Fatalf("provisional gap remains: %+v", gap)
		}
	}
}
func TestHandOver_RepeatFinalizeIdentical(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "skip", "done")
	f.close()
	q := store.New(f.db)
	if _, err := f.db.Exec(f.ctx, `UPDATE gap SET accepted_at=now(), note='accepted' WHERE run_id=$1 AND kind='source_replaced'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	first, err := q.ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := q.GetRunScope(f.ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.close()
	second, err := q.ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := q.GetRunScope(f.ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].AcceptedAt == nil || first[0].ID != second[0].ID || !reflect.DeepEqual(first[0].AcceptedAt, second[0].AcceptedAt) || first[0].Kind != second[0].Kind || first[0].Detail != second[0].Detail || first[0].EvidenceRef == nil || second[0].EvidenceRef == nil || *first[0].EvidenceRef != *second[0].EvidenceRef {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if !bytes.Equal(before.FinalSnapshot, after.FinalSnapshot) || before.FinalHash == nil || after.FinalHash == nil || *before.FinalHash != *after.FinalHash || before.FetchCloseInputHash == nil || after.FetchCloseInputHash == nil || *before.FetchCloseInputHash != *after.FetchCloseInputHash {
		t.Fatal("repeat finalization changed snapshot or input hash")
	}
	f.check("source_replaced", "source_replaced")
}
func TestHandOver_FinalizeFetchedPair(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "done", "done")
	f.close()
	f.check("source_replaced", "source_replaced")
}
func TestHandOver_FinalizeDifferentDate(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-02", "2026-01-02", "done", "done")
	f.close()
	f.check("", "active")
}
func TestHandOver_FinalizeFailedFetch(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "", "gap", "gap")
	f.close()
	f.check("source_replaced", "source_replaced")
	gaps, err := store.New(f.db).ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, gap := range gaps {
		if gap.ItemKey == "vn:vbpl:1" && gap.Kind == "source_unavailable" {
			found = true
		}
	}
	if !found {
		t.Fatal("VBPL fetch gap was removed")
	}
}
func TestHandOver_PlanPrecedence(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "skip", "done")
	entries, err := discover.ExpectedSet(f.ctx, f.db, f.run, f.env.Artifacts)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Key == f.secondary.DocumentKey {
			if e.Skip == nil || e.Skip.Kind != "source_replaced" || e.Skip.Detail != "vn:chinhphu:5 -> vn:vbpl:1" {
				t.Fatalf("entry=%+v", e)
			}
			return
		}
	}
	t.Fatal("secondary omitted")
}

func TestPlan_PrecedenceSkipGapIsProvisional(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "skip", "done")
	if _, err := f.db.Exec(f.ctx, `DELETE FROM gap WHERE run_id=$1 AND stage='fetch'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx, `DELETE FROM run_item WHERE run_id=$1 AND stage='fetch'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	f.run.Snapshot.Recipes = map[string]string{"fetch": f.stage.Recipe(f.run.Snapshot)}
	snapshot, err := json.Marshal(f.run.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, f.run.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	_, err = stage.PlanStage(f.ctx, f.env, f.stage, stage.StageInput{RunID: f.run.ID, Stage: "fetch"}, f.run.Snapshot.Recipes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run_item SET state='gap' WHERE run_id=$1 AND stage='fetch' AND item_key='vn:vbpl:1'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = stage.RunBatch(f.ctx, f.env, f.stage, stage.BatchInput{RunID: f.run.ID, Stage: "fetch", Batch: 0}, 1, stage.BatchConfig{HeartbeatEvery: time.Second, MaxAttempts: 1}, func(...any) {})
	if err != nil {
		t.Fatal(err)
	}
	gaps, err := store.New(f.db).ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil || len(gaps) != 1 {
		t.Fatalf("gaps=%+v err=%v", gaps, err)
	}
	gap := gaps[0]
	if gap.ItemKey != f.secondary.DocumentKey || gap.Kind != "source_replaced" || gap.Detail != "vn:chinhphu:5 -> vn:vbpl:1" || gap.Source != nil || gap.Origin != nil || gap.EvidenceRef != nil {
		t.Fatalf("unexpected provisional gap: %+v", gap)
	}
	f.close()
	final, err := store.New(f.db).ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil || len(final) != 1 {
		t.Fatalf("final gaps=%+v err=%v", final, err)
	}
	result := final[0]
	if result.ItemKey != f.secondary.DocumentKey || result.Kind != gap.Kind || result.Detail != gap.Detail || result.Source == nil || *result.Source != "chinhphu" || result.Origin == nil || *result.Origin != "source_data" || result.EvidenceRef == nil {
		t.Fatalf("provisional=%+v final=%+v", gap, result)
	}
	var refs []string
	for _, path := range []string{"candidate-5.json", "detail/p5-1", "probe-5/p1-1"} {
		refs = append(refs, f.candidate.SHA256+"/"+path)
	}
	if *result.EvidenceRef != strings.Join(refs, ";") {
		t.Fatalf("final gap evidence=%q want=%q", *result.EvidenceRef, strings.Join(refs, ";"))
	}
}

func TestHandOver_FetchedConflictBecomesAmbiguous(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-02", "done", "done")
	f.close()
	f.check("source_identity_ambiguous", "active")
}

func TestHandOver_GapEvidenceExcludesUnrelatedVBPL(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "done", "done", []extraPrimary{{id: "2", number: "42/2026/QD", date: "2026-01-02"}, {id: "3", number: "43/2026/QD", date: "2026-01-01"}, {id: "4", number: "44/2026/QD", date: "2026-01-01"}}...)
	f.close()
	gaps, err := store.New(f.db).ListRunGapsByRun(f.ctx, f.run.ID)
	if err != nil || len(gaps) != 1 || gaps[0].EvidenceRef == nil {
		t.Fatalf("gaps=%+v err=%v", gaps, err)
	}
	refs := strings.Split(*gaps[0].EvidenceRef, ";")
	for _, id := range []string{"1", "2", "3", "4"} {
		want := f.primaryHashes[id] + "/detail.json"
		present := slices.Contains(refs, want)
		if present != (id == "1" || id == "2") {
			t.Errorf("VBPL %s reference present=%v refs=%v", id, present, refs)
		}
	}
}

func TestHandOver_ChangedFinalizedOutcomeFails(t *testing.T) {
	f := newHandOverFixture(t, "2026-01-01", "2026-01-01", "skip", "done")
	f.close()
	if _, err := f.db.Exec(f.ctx, `UPDATE gap SET detail='changed' WHERE run_id=$1 AND kind='source_replaced'`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = f.stage.Finalize(f.ctx, f.env, tx, f.run)
	if err == nil || !strings.Contains(err.Error(), "hand-over outcome changed after finalization") {
		t.Fatalf("error=%v", err)
	}
}
