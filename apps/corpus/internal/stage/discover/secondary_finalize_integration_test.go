//go:build integration

package discover

import (
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func (f *seedFixture) secondaryRun(rel string, keywords ...string) stage.Run {
	f.t.Helper()
	r := f.run(rel, 1)
	list := source.List{Country: "vn", VBPL: source.VBPLSources{SelectionVersion: 1}, Chinhphu: source.ChinhphuSources{IssuerPolicyVersion: 1, CentralIssuerForms: []string{"Chính phủ"}, Keywords: keywords}}
	data, err := json.Marshal(list)
	if err != nil {
		f.t.Fatal(err)
	}
	r.Snapshot.SourceList = data
	snap, err := json.Marshal(r.Snapshot)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `UPDATE run SET snapshot=$2 WHERE id=$1`, r.ID, snap); err != nil {
		f.t.Fatal(err)
	}
	return r
}
func (f *seedFixture) secondaryItem(r stage.Run, keyword string, candidates ...secondaryCandidate) {
	f.t.Helper()
	key := "chinhphu:keyword:" + keyword
	rules := source.ChinhphuSources{Keywords: []string{keyword}}
	definition, err := secondaryItem(key, rules)
	if err != nil {
		f.t.Fatal(err)
	}
	report := secondaryReport{Version: 1, Definition: definition, QueryIdentity: secondaryIdentity(definition), Stable: true, Candidates: candidates}
	recipe := stage.RecipeHash("test", r.Rel, key)
	art, err := f.env.Artifacts.PublishContent(f.ctx, "discover", recipe, func(dir string) error {
		for _, c := range candidates {
			if writeErr := writeJSON(dir, "candidate-"+c.ID+".json", c); writeErr != nil {
				return writeErr
			}
		}
		return writeJSON(dir, "secondary.json", report)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	var id int64
	err = f.db.QueryRow(f.ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('discover',$1,$2,$3,$4,$5) RETURNING id`, key, art.InputHash, recipe, art.Path, art.SHA256).Scan(&id)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'discover',$2,'done',0,$3)`, r.ID, key, id); err != nil {
		f.t.Fatal(err)
	}
}
func TestCeiling_RunWideOverflowFailsDiscover_Integration(t *testing.T) {
	f := newSeedFixture(t)
	r := f.secondaryRun("20990111", "a", "b")
	f.secondaryItem(r, "a", secondaryCandidate{ID: "1"}, secondaryCandidate{ID: "2"})
	f.secondaryItem(r, "b", secondaryCandidate{ID: "2"}, secondaryCandidate{ID: "3"})
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	got, err := New(nil, WithCandidateCeiling(2)).Finalize(f.ctx, f.env, tx, r)
	if err != nil || got.Code != "secondary_candidate_ceiling" || len(got.EvidenceRefs) != 2 {
		t.Fatalf("failure=%+v err=%v", got, err)
	}
	scope, err := store.New(tx).GetRunScope(f.ctx, r.ID)
	if err == nil && scope.SeedsSnapshot != nil {
		t.Fatal("published seeds on overflow")
	}
}
func TestSupport_PerQueryTuplesSurviveSupersession_Integration(t *testing.T) {
	f := newSeedFixture(t)
	candidate := secondaryCandidate{ID: "1", State: "admit_secondary", Number: "142/2026/NĐ-CP", Issuer: "central", IssuerForm: "Chính phủ", Date: "2026-04-30"}
	p := f.secondaryRun("20990112", "a", "b")
	f.item(p, "vbpl:central", true)
	f.secondaryItem(p, "a", candidate)
	f.secondaryItem(p, "b", candidate)
	mustFinalize(t, f, p)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), p.ID, membership.Final, f.read(p).Entries); err != nil {
		t.Fatal(err)
	}
	r := f.secondaryRun("20990113", "a", "b")
	f.item(r, "vbpl:central", true)
	f.secondaryItem(r, "a", candidate)
	candidate.State = "incomplete"
	f.secondaryItem(r, "b", candidate)
	mustFinalize(t, f, r)
	got := f.read(r)
	if len(got.Entries) != 1 || got.Entries[0].Disposition != "active" || len(got.Entries[0].Supports) != 2 {
		t.Fatal("independent support lost")
	}
	records, err := membership.ReadEvidence(f.ctx, store.New(f.db), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, record := range records {
		states[record.Support.RuleValue] = record.State
	}
	if states["chinhphu:keyword:a|1"] != "current" || states["chinhphu:keyword:b|1"] != "retained" {
		t.Fatalf("states=%v", states)
	}
}
