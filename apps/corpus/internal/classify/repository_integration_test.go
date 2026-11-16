//go:build integration

package classify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScreen_DatabaseSettlementAndReplay(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	r, _, f, o := screenFixture(t)
	o.Release = testpg.Release(t)
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES('vn',$1,'paused','{}') RETURNING id`, o.Release).Scan(&o.RunID); err != nil {
		t.Fatal(err)
	}
	r.Repository = &PGRepository{Pool: db}
	f.beforeCreate = func(requests []batchapi.Request) {
		var id, state string
		var reserve int64
		if queryErr := db.QueryRow(ctx, `SELECT id,state,reserved_micro_usd FROM submission WHERE run_id=$1`, o.RunID).Scan(&id, &state, &reserve); queryErr != nil {
			t.Fatal(queryErr)
		}
		if state != "preparing" || reserve != 1029 || requests[0].CustomID != id+"-000000" {
			t.Fatal("submission was not reserved before create")
		}
		if _, statErr := os.Stat(filepath.Join(r.IntentDir, "submissions", id+".json")); statErr != nil {
			t.Fatal(statErr)
		}
		var message anthropic.Message
		if decodeErr := json.Unmarshal([]byte(`{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"codes\":[\"1.1\"]}"}],"usage":{"input_tokens":100,"output_tokens":10}}`), &message); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		f.results = []batchapi.Result{{CustomID: requests[0].CustomID, Type: "succeeded", Message: message}}
	}
	if _, err = r.Screen(ctx, o, documents(), families()); err != nil {
		t.Fatal(err)
	}
	var state string
	var cost int64
	var settled bool
	if err = db.QueryRow(ctx, `SELECT state,cost_micro_usd,cost_settled FROM submission WHERE run_id=$1`, o.RunID).Scan(&state, &cost, &settled); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || cost != 8 || !settled {
		t.Fatalf("state %s cost %d settled %t", state, cost, settled)
	}
	if _, err = r.Screen(ctx, o, documents(), families()); err != nil {
		t.Fatal(err)
	}
	if f.creates != 1 || f.downloads != 1 {
		t.Fatal("replayed provider operations after settlement")
	}
}

func TestRepository_SerializesBudgetAdmission(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	first := &PGRepository{Pool: db}
	second := &PGRepository{Pool: db}
	unlock, err := first.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if release, lockErr := second.Lock(waitCtx); lockErr == nil {
		release()
		unlock()
		t.Fatal("two commands acquired budget admission lock")
	}
	unlock()
	release, err := second.Lock(ctx)
	if err != nil {
		t.Fatal("budget lock was not released", err)
	}
	release()
}
