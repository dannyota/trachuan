//go:build integration

package classify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func submissionDatabase(t *testing.T) (*pgxpool.Pool, int64, string) {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	id, rel := submissionRun(t, db)
	return db, id, rel
}

func submissionRun(t *testing.T, db *pgxpool.Pool) (int64, string) {
	t.Helper()
	var id int64
	rel := testpg.Release(t)
	if err := db.QueryRow(context.Background(), `INSERT INTO run(country,rel,state,snapshot) VALUES('vn',$1,'paused','{}') RETURNING id`, rel).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id, rel
}

func TestRebuild_DatabaseAbandonedReplacement(t *testing.T) {
	ctx := context.Background()
	db, id, rel := submissionDatabase(t)
	r, _, client, o := screenFixture(t)
	r.Repository = &PGRepository{Pool: db}
	o.RunID, o.Release = id, rel
	o.Recipe, _ = NewRecipe("claude-sonnet-5-5", o.Recipe.TaxonomyHash)
	o.Documents = 124
	docs := make([]Document, 124)
	for i := range docs {
		docs[i] = Document{Key: fmt.Sprintf("vn:vbpl:%d", i+1), Title: "Synthetic legal title"}
	}
	client.input = 40000
	client.beforeCreate = nil
	client.createErr = errors.New("synthetic lost create response")
	if _, err := r.Screen(ctx, o, docs, families()); err == nil {
		t.Fatal("expected uncertain submission")
	}
	var abandonedID string
	if err := db.QueryRow(ctx, `SELECT id FROM submission WHERE run_id=$1`, id).Scan(&abandonedID); err != nil {
		t.Fatal(err)
	}
	created := r.Now()
	r.Now = func() time.Time { return created.Add(25 * time.Hour) }
	if err := r.Reconcile(ctx, abandonedID, true); err != nil {
		t.Fatal(err)
	}
	client.createErr = nil
	client.ended = false
	if _, err := r.Screen(ctx, o, docs, families()); err == nil {
		t.Fatal("expected interrupted replacement polling")
	}
	if _, err := db.Exec(ctx, `DELETE FROM submission`); err != nil {
		t.Fatal(err)
	}
	if err := r.Rebuild(ctx); err != nil {
		t.Fatalf("rebuild abandoned attempt and replacement: %v", err)
	}
	var count int
	var state string
	var reserve int64
	var settled bool
	if err := db.QueryRow(ctx, `SELECT state,reserved_micro_usd,cost_settled FROM submission WHERE id=$1`, abandonedID).Scan(&state, &reserve, &settled); err != nil {
		t.Fatal(err)
	}
	if state != "abandoned" || reserve != 7499520 || settled {
		t.Fatalf("abandoned state %s reserve %d settled %t", state, reserve, settled)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM submission`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("restored count %d error %v", count, err)
	}
	t.Run("budget includes both attempts", func(t *testing.T) {
		o.RunID, o.Release = submissionRun(t, db)
		o.Recipe, _ = NewRecipe("claude-haiku-5-5", o.Recipe.TaxonomyHash)
		o.Documents = 1
		o.Out = t.TempDir()
		client.input = 100
		before := client.creates
		if _, err := r.Screen(ctx, o, documents(), families()); err == nil || client.creates != before {
			t.Fatal("admitted without abandoned reservation")
		}
	})
}

func TestRebuild_DatabaseAtomicAndBlocksIncompleteAdmission(t *testing.T) {
	ctx := context.Background()
	db, id, rel := submissionDatabase(t)
	r, _, client, o := screenFixture(t)
	r.Repository = &PGRepository{Pool: db}
	o.RunID, o.Release = id, rel
	for i := 0; i < 2; i++ {
		s := Submission{ID: strings.Repeat(string(rune('a'+i)), 32), RunID: id, Country: "vn", Release: rel, ScreenHash: strings.Repeat(string(rune('a'+i)), 64), Recipe: o.Recipe, Items: []Item{{Key: "vn:vbpl:1", InputTokens: 100}}, State: "preparing", CreatedAt: r.Now(), Budget: "pilot", BudgetMicroUSD: 15000000, ReserveMicroUSD: 1029}
		if i == 1 {
			s.RunID = 9223372036854775807
		}
		s.ItemsHash = itemsHash(s.Items)
		if err := r.writeIntent(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Rebuild(ctx); err == nil {
		t.Fatal("accepted missing run in recovered attempt")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM submission`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial reconstruction left %d rows: %v", count, err)
	}
	client.beforeCreate = nil
	if _, err := r.Screen(ctx, o, documents(), families()); err == nil || client.creates != 0 {
		t.Fatal("admission ignored incomplete reconstruction")
	}
}
