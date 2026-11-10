//go:build integration

package expand

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestPredecessor_HashMismatchFails(t *testing.T) {
	f := newFixture(t)
	old := f.run("20990101")
	f.phase(old, membership.Final, testContext("2", "1"))
	r := f.run("20990103")
	f.phase(r, membership.Seeds, testSeed("1"))
	if _, err := f.db.Exec(f.ctx, `UPDATE run_scope SET predecessor_run_id=$2,predecessor_phase='final',predecessor_snapshot_hash='wrong' WHERE run_id=$1`, r.ID, old.ID); err != nil {
		t.Fatal(err)
	}
	f.item(r, "vn:vbpl:1", "gap", nil)
	if err := f.close(r); err == nil {
		t.Fatal("accepted mismatched predecessor hash")
	}
}

func TestPredecessor_ExpandAndFetchUseRecord(t *testing.T) {
	f := newFixture(t)
	old := f.run("20990101")
	f.phase(old, membership.Final, testContext("2", "1"))
	snap, err := membership.ReadPhase(f.ctx, store.New(f.db), old.ID, membership.Final)
	if err != nil {
		t.Fatal(err)
	}
	r := f.run("20990103")
	f.phase(r, membership.Seeds, testSeed("1"))
	if _, err = f.db.Exec(f.ctx, `UPDATE run_scope SET predecessor_run_id=$2,predecessor_phase='final',predecessor_snapshot_hash=$3 WHERE run_id=$1`, r.ID, old.ID, snap.Hash); err != nil {
		t.Fatal(err)
	}
	newer := f.run("20990102")
	f.phase(newer, membership.Final, testContext("3", "1"))
	f.item(r, "vn:vbpl:1", "gap", nil)
	if err = f.close(r); err != nil {
		t.Fatal(err)
	}
	got, err := membership.ReadPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded)
	if err != nil || len(got.Entries) != 2 || got.Entries[1].DocumentKey != "vn:vbpl:2" {
		t.Fatalf("snapshot=%#v err=%v", got, err)
	}
}

func TestRemoval_RelationAcceptanceAndReadmission(t *testing.T) {
	for _, tc := range []struct {
		name              string
		accepted, current bool
		want              string
	}{
		{"unaccepted", false, false, "withdrawn"},
		{"accepted", true, false, ""},
		{"accepted and readmitted", true, true, "active"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			old := f.run("20990101")
			removed := testContext("2", "1")
			removed.Disposition = "withdrawn"
			f.phase(old, membership.Final, removed)
			if _, err := f.db.Exec(f.ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail,accepted_at) VALUES ($1,'fetch','vn:vbpl:2','withdrawn','',CASE WHEN $2 THEN now() ELSE NULL END)`, old.ID, tc.accepted); err != nil {
				t.Fatal(err)
			}
			r := f.run("20990102")
			f.phase(r, membership.Seeds, testSeed("1"))
			if tc.current {
				obs := Observation{Version: 1, SeedKey: "vn:vbpl:1", Targets: []Target{{TargetKey: "vn:vbpl:2", RawCode: "A", Response: "detail.json", ResponseSHA256: "response"}}}
				f.item(r, "vn:vbpl:1", "done", &obs)
			} else {
				f.item(r, "vn:vbpl:1", "gap", nil)
			}
			if err := f.close(r); err != nil {
				t.Fatal(err)
			}
			snap, err := membership.ReadPhase(f.ctx, store.New(f.db), r.ID, membership.Expanded)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			for _, e := range snap.Entries {
				if e.DocumentKey == "vn:vbpl:2" {
					got = e.Disposition
				}
			}
			if got != tc.want {
				t.Fatalf("disposition=%q want=%q entries=%#v", got, tc.want, snap.Entries)
			}
		})
	}
}
