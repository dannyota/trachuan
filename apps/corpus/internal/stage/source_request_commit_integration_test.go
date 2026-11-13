//go:build integration

package stage

import (
	"context"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func TestCommit_SourceRequestsAreAtomic(t *testing.T) {
	ctx := context.Background()
	db, id := setup(t)
	if _, err := db.Exec(ctx, `UPDATE run_item SET stage='discover' WHERE run_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	status := 200
	record := sourcegate.RequestRecord{Source: "vbpl", Endpoint: "list", Attempt: 1, StartedAt: time.Now().UTC(), DurationMS: 3, Status: &status, Outcome: "ok"}
	c := Commit{RunID: id, Stage: "discover", ItemKey: "item", Generation: 2, Attempt: 1, RecipeHash: "recipe", State: "done", Requests: []sourcegate.RequestRecord{record}}
	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM run_source_request WHERE run_id=$1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	c.Generation = 1
	if applied, err := CommitItem(ctx, db, c); err != nil || applied || count() != 0 {
		t.Fatalf("stale applied=%v err=%v rows=%d", applied, err, count())
	}
	c.Generation = 2
	c.Requests = append(c.Requests, sourcegate.RequestRecord{Source: "vbpl", Endpoint: "invalid", Attempt: 1, StartedAt: time.Now(), Outcome: "ok"})
	if _, err := CommitItem(ctx, db, c); err == nil {
		t.Fatal("invalid request was committed")
	}
	var state string
	if err := db.QueryRow(ctx, `SELECT state FROM run_item WHERE run_id=$1 AND stage='discover'`, id).Scan(&state); err != nil || state != "pending" || count() != 0 {
		t.Fatalf("state=%s rows=%d err=%v", state, count(), err)
	}
	c.Requests = c.Requests[:1]
	c.Requests = append(c.Requests,
		sourcegate.RequestRecord{Source: "vbpl", Endpoint: "diagram", Attempt: 1, StartedAt: time.Now(), Outcome: "cancelled"},
		sourcegate.RequestRecord{Source: "chinhphu", Endpoint: "search", Attempt: 2, StartedAt: time.Now(), Status: &status, Outcome: "invalid_body"},
	)
	if applied, err := CommitItem(ctx, db, c); err != nil || !applied || count() != 3 {
		t.Fatalf("applied=%v rows=%d err=%v", applied, count(), err)
	}
	var valid int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM run_source_request WHERE run_id=$1 AND ((endpoint='list' AND attempt=1 AND status=200 AND outcome='ok') OR (endpoint='diagram' AND status IS NULL AND outcome='cancelled') OR (endpoint='search' AND attempt=2 AND status=200 AND outcome='invalid_body'))`, id).Scan(&valid); err != nil || valid != 3 {
		t.Fatalf("valid rows=%d err=%v", valid, err)
	}
}
