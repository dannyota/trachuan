//go:build integration

package discover

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestPredecessor_SkipsRunsWithoutFinal(t *testing.T) {
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	entries := []membership.Entry{seedEntry("1", seedTuple("agency", "62"))}
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, entries); err != nil {
		t.Fatal(err)
	}
	newer := f.run("20990102", 1)
	f.previous(newer, seedEntry("2", seedTuple("agency", "62")))
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	s := f.read(r)
	if len(s.Entries) != 1 || s.Entries[0].DocumentKey != "vn:vbpl:1" {
		t.Fatalf("entries = %#v, want older final membership", s.Entries)
	}
}

func TestPredecessor_RecordedAtDiscoveryClose(t *testing.T) {
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	entries := []membership.Entry{seedEntry("1", seedTuple("agency", "62"))}
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, entries); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	scope, err := store.New(f.db).GetRunScope(f.ctx, r.ID)
	_, hash, e := membership.CanonicalSnapshot(entries)
	if err != nil || e != nil || scope.PredecessorRunID == nil || *scope.PredecessorRunID != old.ID || scope.PredecessorPhase == nil || *scope.PredecessorPhase != "final" || scope.PredecessorSnapshotHash == nil || *scope.PredecessorSnapshotHash != hash {
		t.Fatalf("scope=%+v err=%v hash error=%v", scope, err, e)
	}
}

func removalFixture(t *testing.T, accepted, incomplete bool) (*seedFixture, stage.Run) {
	t.Helper()
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	entry := seedEntry("1", seedTuple("agency", "62"))
	entry.Disposition = "withdrawn"
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, []membership.Entry{entry}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'fetch','vn:vbpl:1','gap',0)`, old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(f.ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail,accepted_at) VALUES ($1,'fetch','vn:vbpl:1','withdrawn','',CASE WHEN $2 THEN now() ELSE NULL END)`, old.ID, accepted); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", !incomplete)
	mustFinalize(t, f, r)
	return f, r
}

func TestRemoval_UnacceptedCarries(t *testing.T) {
	f, r := removalFixture(t, false, false)
	s := f.read(r)
	if len(s.Entries) != 1 || s.Entries[0].Disposition != "withdrawn" || f.state(r, "1", "agency", "62") != "removed" {
		t.Fatalf("entries=%#v", s.Entries)
	}
}
func TestRemoval_AcceptedIsDropped(t *testing.T) {
	f, r := removalFixture(t, true, false)
	if s := f.read(r); len(s.Entries) != 0 {
		t.Fatalf("entries=%#v", s.Entries)
	}
}
func TestRemoval_IncompleteObservationDoesNotRevive(t *testing.T) {
	f, r := removalFixture(t, false, true)
	if s := f.read(r); len(s.Entries) != 1 || s.Entries[0].Disposition != "withdrawn" || f.state(r, "1", "agency", "62") != "removed" {
		t.Fatalf("entries=%#v", s.Entries)
	}
}

func TestPredecessor_LegacyTupleInFinalFollowsNormalRetention(t *testing.T) {
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	query := "vbpl:sweep@old"
	tuple := seedTuple("agency", "62|908")
	tuple.Evidence.QueryIdentity = &query
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, []membership.Entry{seedEntry("1", tuple)}); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", false)
	f.listing(r, "1", source.SelectionRule{Kind: "term", Value: "new"})
	mustFinalize(t, f, r)
	if got := f.state(r, "1", "agency", "62|908"); got != "retained" {
		t.Fatalf("state=%q", got)
	}
}

func TestPredecessor_ExpandedOnlyIsSkipped(t *testing.T) {
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, []membership.Entry{seedEntry("1", seedTuple("agency", "62"))}); err != nil {
		t.Fatal(err)
	}
	newer := f.run("20990102", 1)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), newer.ID, membership.Expanded, []membership.Entry{seedEntry("2", seedTuple("agency", "62"))}); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	if s := f.read(r); len(s.Entries) != 1 || s.Entries[0].DocumentKey != "vn:vbpl:1" {
		t.Fatalf("entries=%#v", s.Entries)
	}
}

func TestPredecessor_FrozenNoneStaysNone(t *testing.T) {
	f := newSeedFixture(t)
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	old := f.run("20990101", 1)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, []membership.Entry{seedEntry("1", seedTuple("agency", "62"))}); err != nil {
		t.Fatal(err)
	}
	mustFinalize(t, f, r)
	if s := f.read(r); len(s.Entries) != 0 {
		t.Fatalf("entries=%#v", s.Entries)
	}
}

func TestRemoval_CurrentDirectReadmits(t *testing.T) {
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	removed := seedEntry("1", seedTuple("agency", "62"))
	removed.Disposition = "withdrawn"
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, []membership.Entry{removed}); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990102", 1)
	f.item(r, "vbpl:central", false)
	f.listing(r, "1", source.SelectionRule{Kind: "agency", Value: "62"})
	mustFinalize(t, f, r)
	if s := f.read(r); len(s.Entries) != 1 || s.Entries[0].Disposition != "active" || len(s.Entries[0].Supports) != 1 {
		t.Fatalf("entries=%#v", s.Entries)
	}
}

func TestPredecessor_FinalPreferredOverNewerLegacy(t *testing.T) {
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, []membership.Entry{seedEntry("1", seedTuple("agency", "62"))}); err != nil {
		t.Fatal(err)
	}
	legacy := f.run("20990102", 0)
	if _, err := f.db.Exec(f.ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'fetch','vn:vbpl:2','done',0)`, legacy.ID); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	s := f.read(r)
	if len(s.Entries) != 1 || s.Entries[0].DocumentKey != "vn:vbpl:1" {
		t.Fatalf("entries=%#v", s.Entries)
	}
}

func TestPredecessor_ReselectedAfterSeedsCleared(t *testing.T) {
	f := newSeedFixture(t)
	old := f.run("20990101", 1)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), old.ID, membership.Final, nil); err != nil {
		t.Fatal(err)
	}
	r := f.run("20990103", 1)
	f.item(r, "vbpl:central", false)
	mustFinalize(t, f, r)
	newer := f.run("20990102", 1)
	if err := membership.PublishPhase(f.ctx, store.New(f.db), newer.ID, membership.Final, nil); err != nil {
		t.Fatal(err)
	}
	if err := membership.ClearPhase(f.ctx, store.New(f.db), r.ID, membership.Seeds); err != nil {
		t.Fatal(err)
	}
	mustFinalize(t, f, r)
	scope, err := store.New(f.db).GetRunScope(f.ctx, r.ID)
	if err != nil || scope.PredecessorRunID == nil || *scope.PredecessorRunID != newer.ID {
		t.Fatalf("scope=%+v err=%v", scope, err)
	}
}
