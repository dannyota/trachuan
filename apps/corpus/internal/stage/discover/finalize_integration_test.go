//go:build integration

package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type seedFixture struct {
	t   *testing.T
	ctx context.Context
	db  *pgxpool.Pool
	env stage.Env
}

func newSeedFixture(t *testing.T) *seedFixture {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return &seedFixture{t, ctx, db, stage.Env{DB: db, Artifacts: artifact.Store{Root: t.TempDir()}}}
}
func (f *seedFixture) run(rel string, version int, names ...string) stage.Run {
	f.t.Helper()
	list := source.List{Country: "vn", VBPL: source.VBPLSources{SelectionVersion: version, NamedNumbers: names}}
	data, err := json.Marshal(list)
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
func (f *seedFixture) item(r stage.Run, key string, stable bool, ids ...string) {
	f.t.Helper()
	report := scopeReport{Version: 2, Stable: stable}
	catalog := catalogFile{Complete: stable, IDs: ids}
	art, err := f.env.Artifacts.PublishContent(f.ctx, "discover", stage.RecipeHash("test", r.Rel, key), func(dir string) error {
		if err := writeJSON(dir, "report.json", report); err != nil {
			return err
		}
		return writeJSON(dir, "catalog.json", catalog)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	var id int64
	err = f.db.QueryRow(f.ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('discover',$1,$2,$3,$4,$5) RETURNING id`, key, art.InputHash, stage.RecipeHash("test", r.Rel, key), art.Path, art.SHA256).Scan(&id)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'discover',$2,'done',0,$3)`, r.ID, key, id); err != nil {
		f.t.Fatal(err)
	}
}
func (f *seedFixture) listing(r stage.Run, id string, rules ...source.SelectionRule) {
	f.t.Helper()
	item := selectedItem{ID: id, Rules: rules, Page: pageReference{Pass: 2, Page: 1, SHA256: "response", Path: "pages/p2-1.json.gz"}}
	b, err := json.Marshal(item)
	if err != nil {
		f.t.Fatal(err)
	}
	identity := source.CentralListingIdentity("vbpl", "TRUNG_UONG", 1000)
	if _, err = f.db.Exec(f.ctx, `INSERT INTO listing(run_id,query_key,item_key,payload) VALUES ($1,$2,$3,$4)`, r.ID, identity, "vn:vbpl:"+id, b); err != nil {
		f.t.Fatal(err)
	}
}
func (f *seedFixture) previous(r stage.Run, entries ...membership.Entry) {
	f.t.Helper()
	if err := membership.PublishPhase(f.ctx, store.New(f.db), r.ID, membership.Seeds, entries); err != nil {
		f.t.Fatal(err)
	}
}
func (f *seedFixture) finalPrevious(r stage.Run, entries ...membership.Entry) {
	f.t.Helper()
	if err := membership.PublishPhase(f.ctx, store.New(f.db), r.ID, membership.Final, entries); err != nil {
		f.t.Fatal(err)
	}
}
func seedEntry(id string, tuples ...membership.SupportTuple) membership.Entry {
	return membership.Entry{DocumentKey: "vn:vbpl:" + id, Source: "vbpl", SourceID: id, Class: "primary", Disposition: "active", Supports: tuples}
}
func seedTuple(kind, value string) membership.SupportTuple {
	return membership.SupportTuple{RuleKind: kind, RuleValue: value, Evidence: membership.EvidenceRef{ResponseSHA256: "old", ArtifactHash: "old", Path: "report.json"}}
}
func (f *seedFixture) finalize(r stage.Run) error {
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
func (f *seedFixture) read(r stage.Run) membership.PhaseSnapshot {
	f.t.Helper()
	s, err := membership.ReadPhase(f.ctx, store.New(f.db), r.ID, membership.Seeds)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}
func (f *seedFixture) state(r stage.Run, id, kind, value string) string {
	f.t.Helper()
	var state string
	err := f.db.QueryRow(f.ctx, `SELECT support_state FROM run_membership_evidence WHERE run_id=$1 AND document_key=$2 AND rule_kind=$3 AND rule_value=$4`, r.ID, "vn:vbpl:"+id, kind, value).Scan(&state)
	if err != nil {
		f.t.Fatal(err)
	}
	return state
}
func mustFinalize(t *testing.T, f *seedFixture, r stage.Run) {
	t.Helper()
	if err := f.finalize(r); err != nil {
		t.Fatal(err)
	}
}
func TestFinalize_SeedsFromCurrentSupports(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", true, "1")
	f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"}, source.SelectionRule{Kind: "term", Value: "law"})
	mustFinalize(t, f, r)
	s := f.read(r)
	if len(s.Entries) != 1 || len(s.Entries[0].Supports) != 2 || s.Entries[0].Disposition != "active" {
		t.Fatalf("entries = %#v", s.Entries)
	}
}
func TestScopeRemoved_SelectionLossVsCatalogAbsence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		old, now []string
		want     string
	}{{"present", nil, []string{"1"}, "scope_removed"}, {"twice absent", nil, nil, "withdrawn"}, {"once absent", []string{"1"}, nil, "active"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSeedFixture(t)
			p := f.run("20990101", 1)
			f.item(p, "vbpl:central", true, tc.old...)
			f.finalPrevious(p, seedEntry("1", seedTuple("agency", "62")))
			r := f.run("20990102", 1)
			f.item(r, "vbpl:central", true, tc.now...)
			mustFinalize(t, f, r)
			s := f.read(r)
			if len(s.Entries) != 1 || s.Entries[0].Disposition != tc.want {
				t.Fatalf("entries = %#v", s.Entries)
			}
		})
	}
}
func TestMembership_IndependentSupportRetained(t *testing.T) {
	f := newSeedFixture(t)
	p := f.run("20990101", 1)
	f.finalPrevious(p, seedEntry("1", seedTuple("agency", "62"), seedTuple("term", "law")))
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", false)
	f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"})
	mustFinalize(t, f, r)
	if got := f.state(r, "1", "term", "law"); got != "retained" {
		t.Fatal(got)
	}
	if len(f.read(r).Entries[0].Supports) < 2 {
		t.Fatal("lost independent support")
	}
}
func TestFinalize_IncompleteCatalogRetains(t *testing.T) {
	f := newSeedFixture(t)
	p := f.run("20990101", 1)
	f.finalPrevious(p, seedEntry("1", seedTuple("agency", "62")))
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	if got := f.state(r, "1", "agency", "62"); got != "retained" {
		t.Fatal(got)
	}
}
func TestFinalize_IncompleteNamedLookupRetains(t *testing.T) {
	f := newSeedFixture(t)
	const name = "71/2025/QH15"
	p := f.run("20990101", 1, name)
	f.item(p, "vbpl:central", true)
	f.finalPrevious(p, seedEntry("1", seedTuple("named", name)))
	r := f.run("20990102", 1, name)
	f.item(r, "vbpl:central", true)
	f.item(r, "vbpl:named:"+name, false)
	mustFinalize(t, f, r)
	if got := f.state(r, "1", "named", name); got != "retained" {
		t.Fatal(got)
	}
	if got := f.read(r).Entries[0].Disposition; got != "active" {
		t.Fatal(got)
	}
}
func TestFinalize_Idempotent(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", true, "1")
	f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"})
	mustFinalize(t, f, r)
	first := f.read(r)
	mustFinalize(t, f, r)
	if !reflect.DeepEqual(first, f.read(r)) {
		t.Fatal("changed snapshot")
	}
	f.listing(r, "2", source.SelectionRule{Kind: "agency", Value: "62"})
	if err := f.finalize(r); err == nil {
		t.Fatal("accepted changed frozen inputs")
	}
}
func TestFinalize_LegacySnapshotNoop(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 0)
	mustFinalize(t, f, r)
	var n int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM run_scope WHERE run_id=$1`, r.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("scope count = %d, err = %v", n, err)
	}
}
func TestExpectedSet_ExpandedWhenStaged(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	r.Snapshot.Stages = []string{"discover", "expand", "fetch"}
	entries := []membership.Entry{seedEntry("1"), seedEntry("2"), seedEntry("3"), seedEntry("4")}
	entries[1].Disposition = "withdrawn"
	entries[2].Disposition = "scope_removed"
	entries[3].Disposition = "source_replaced"
	k := "vn:vbpl:1"
	entries[3].ReplacementKey = &k
	if err := membership.PublishPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded, entries); err != nil {
		t.Fatal(err)
	}
	got, err := ExpectedSet(f.ctx, f.db, r)
	want := []Expected{document("1", ""), document("2", "withdrawn"), document("3", "scope_removed")}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v, err %v", got, want, err)
	}
}
func TestFinalize_LegacyPriorRunMigration(t *testing.T) {
	f := newSeedFixture(t)
	p := f.run("20990101", 0)
	f.item(p, "vbpl:sweep", true)
	if _, err := f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'fetch','vn:vbpl:1','done',0)`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx, `INSERT INTO listing(run_id,query_key,item_key,payload) VALUES ($1,'vbpl:sweep@old','vn:vbpl:1','{}')`, p.ID); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", true, "1")
	mustFinalize(t, f, r)
	s := f.read(r)
	if len(s.Entries) != 1 || s.Entries[0].Disposition != "scope_removed" || f.state(r, "1", "agency", "62|908") != "removed" {
		t.Fatalf("entries = %s", fmt.Sprint(s.Entries))
	}
}

func TestFinalize_RejectsChangedSupportStateAfterFreeze(t *testing.T) {
	f := newSeedFixture(t)
	p := f.run("20990101", 1)
	f.finalPrevious(p, seedEntry("1", seedTuple("term", "law")))
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", false)
	f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"})
	mustFinalize(t, f, r)
	if _, err := f.db.Exec(f.ctx, `UPDATE run_membership_evidence SET support_state='removed' WHERE run_id=$1 AND rule_kind='term'`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.finalize(r); err == nil {
		t.Fatal("accepted changed evidence after frozen snapshot")
	}
}
func TestFinalize_IgnoresNonDirectPriorSupport(t *testing.T) {
	f := newSeedFixture(t)
	p := f.run("20990101", 1)
	f.finalPrevious(p, seedEntry("1", seedTuple("relation", "seed|code")), seedEntry("2", seedTuple("term", "law"), seedTuple("relation", "seed|code")), seedEntry("3", seedTuple("secondary_probe", "doc")))
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	s := f.read(r)
	if len(s.Entries) != 1 || s.Entries[0].DocumentKey != "vn:vbpl:2" || len(s.Entries[0].Supports) != 1 {
		t.Fatalf("entries = %#v", s.Entries)
	}
}

func TestFinalize_NamedEntriesSharingLookupKeepBothSupports(t *testing.T) {
	f := newSeedFixture(t)
	a, b := "71/2025/QH15", "71/2025/QH15@2025-01-01"
	r := f.run("20990103", 1, a, b)
	f.item(r, "vbpl:central", true, "1")
	for _, name := range []string{a, b} {
		f.namedItem(r, name, "1")
	}
	item := selectedItem{ID: "1", Rules: []source.SelectionRule{{Kind: "named", Value: a}}, Page: pageReference{Pass: 2, Page: 1, SHA256: "response", Path: "pages/p2-1.json.gz"}}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	identity := source.NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", a)
	if _, err = f.db.Exec(f.ctx, `INSERT INTO listing(run_id,query_key,item_key,payload) VALUES ($1,$2,'vn:vbpl:1',$3)`, r.ID, identity, payload); err != nil {
		t.Fatal(err)
	}
	mustFinalize(t, f, r)
	if s := f.read(r); len(s.Entries) != 1 || len(s.Entries[0].Supports) != 2 {
		t.Fatalf("entries = %#v", s.Entries)
	}
}
func (f *seedFixture) namedItem(r stage.Run, name string, ids ...string) {
	f.t.Helper()
	entry, err := source.ParseNamedNumber(name)
	if err != nil {
		f.t.Fatal(err)
	}
	identity := source.NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", entry.Number)
	ordinal := 1
	date := "2025-01-01"
	number := entry.Number
	items := make([]lookup.Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, lookup.Item{Source: "vbpl", SourceID: id, RawNumber: &number, IssueDate: &date, PageRef: "response"})
	}
	result := lookup.Result{Version: 1, Items: items, Evidence: lookup.Evidence{QueryIdentity: identity, Completeness: lookup.Complete, PagesFetched: []lookup.Page{{Ordinal: &ordinal, SHA256: "response", Path: "pages/p2-1.json.gz"}}}}
	report := scopeReport{Version: 2, Stable: true, QueryIdentity: identity, Passes: []lookup.Result{result, result}}
	key := "vbpl:named:" + name
	recipe := stage.RecipeHash("test", r.Rel, key)
	art, err := f.env.Artifacts.PublishContent(f.ctx, "discover", recipe, func(dir string) error {
		if writeErr := writeJSON(dir, "report.json", report); writeErr != nil {
			return writeErr
		}
		return writeJSON(dir, "lookup.json", result)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	var aid int64
	err = f.db.QueryRow(f.ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('discover',$1,$2,$3,$4,$5) RETURNING id`, key, art.InputHash, recipe, art.Path, art.SHA256).Scan(&aid)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'discover',$2,'done',0,$3)`, r.ID, key, aid); err != nil {
		f.t.Fatal(err)
	}
}

func TestFinalize_RepeatDoesNotOverwriteLaterMembership(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", true, "1")
	f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"})
	mustFinalize(t, f, r)
	if _, err := f.db.Exec(f.ctx, `UPDATE run_document SET disposition='withdrawn' WHERE run_id=$1`, r.ID); err != nil {
		t.Fatal(err)
	}
	mustFinalize(t, f, r)
	var got string
	if err := f.db.QueryRow(f.ctx, `SELECT disposition FROM run_document WHERE run_id=$1`, r.ID).Scan(&got); err != nil || got != "withdrawn" {
		t.Fatalf("disposition = %q, err = %v", got, err)
	}
}

func TestFinalize_AmbiguousNamedLookupRetainsEligiblePrior(t *testing.T) {
	f := newSeedFixture(t)
	const name = "71/2025/QH15"
	p := f.run("20990101", 1, name)
	f.item(p, "vbpl:central", true)
	f.finalPrevious(p, seedEntry("1", seedTuple("named", name)))
	r := f.run("20990102", 1, name)
	f.item(r, "vbpl:central", true)
	f.namedItem(r, name, "1", "2")
	mustFinalize(t, f, r)
	if got := f.state(r, "1", "named", name); got != "retained" {
		t.Fatal(got)
	}
	if got := f.read(r).Entries[0].Disposition; got != "active" {
		t.Fatal(got)
	}
}

func TestFinalize_FrozenDiscoveryIgnoresRelationEvidence(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", true, "1")
	f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"})
	mustFinalize(t, f, r)
	relation := membership.EvidenceRecord{DocumentKey: "vn:vbpl:1", Stage: "discover", ItemKey: "vbpl:central", State: "removed", Support: seedTuple("relation", "vn:vbpl:2|A")}
	if err := membership.RecordEvidence(f.ctx, store.New(f.db), r.ID, relation); err != nil {
		t.Fatal(err)
	}
	mustFinalize(t, f, r)
	if _, err := f.db.Exec(f.ctx, `UPDATE run_membership_evidence SET support_state='removed' WHERE run_id=$1 AND rule_kind='agency'`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.finalize(r); err == nil {
		t.Fatal("accepted changed direct evidence")
	}
}

func TestExpectedSet_RequiresExpandedWhenStaged(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	r.Snapshot.Stages = []string{"discover", "expand", "fetch"}
	f.previous(r, seedEntry("1"))
	if _, err := ExpectedSet(f.ctx, f.db, r); err == nil {
		t.Fatal("accepted unfrozen expanded membership")
	}
}
func TestExpectedSet_UsesSeedsWhenExpandNotStaged(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	f.previous(r, seedEntry("1"))
	if err := membership.PublishPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded, []membership.Entry{seedEntry("2")}); err != nil {
		t.Fatal(err)
	}
	got, err := ExpectedSet(f.ctx, f.db, r)
	if err != nil || len(got) != 1 || got[0].Key != "vn:vbpl:1" {
		t.Fatalf("expected=%#v err=%v", got, err)
	}
}

func TestFinalize_CurrentRuleSupersedesOlderEvidence(t *testing.T) {
	f := newSeedFixture(t)
	for _, rel := range []string{"20990101", "20990102", "20990103"} {
		r := f.run(rel, 1)
		f.item(r, "vbpl:central", true, "1", rel)
		f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"})
		mustFinalize(t, f, r)
		s := f.read(r)
		if len(s.Entries) != 1 || len(s.Entries[0].Supports) != 1 {
			t.Fatalf("%s entries=%#v", rel, s.Entries)
		}
		records, err := membership.ReadEvidence(f.ctx, store.New(f.db), r.ID)
		if publishErr := membership.PublishPhase(f.ctx, store.New(f.db), r.ID, membership.Final, s.Entries); publishErr != nil {
			t.Fatal(publishErr)
		}
		if err != nil || len(records) != 1 || records[0].State != "current" {
			t.Fatalf("%s records=%#v err=%v", rel, records, err)
		}
	}
}
func TestFinalize_DisappearedRuleDoesNotAccumulate(t *testing.T) {
	f := newSeedFixture(t)
	p := f.run("20990101", 1)
	f.item(p, "vbpl:central", true, "1")
	f.listing(p, "1", source.SelectionRule{Kind: "agency", Value: "62"})
	mustFinalize(t, f, p)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), p.ID, membership.Final, f.read(p).Entries); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"20990102", "20990103"} {
		r := f.run(rel, 1)
		f.item(r, "vbpl:central", true, "1")
		mustFinalize(t, f, r)
		s := f.read(r)
		records, err := membership.ReadEvidence(f.ctx, store.New(f.db), r.ID)
		if publishErr := membership.PublishPhase(f.ctx, store.New(f.db), r.ID, membership.Final, s.Entries); publishErr != nil {
			t.Fatal(publishErr)
		}
		if err != nil || len(s.Entries) != 1 || len(s.Entries[0].Supports) != 1 || len(records) != 1 || records[0].State != "removed" {
			t.Fatalf("%s entries=%#v records=%#v err=%v", rel, s.Entries, records, err)
		}
	}
}
