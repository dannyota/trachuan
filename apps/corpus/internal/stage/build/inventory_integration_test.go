//go:build integration

package build

import (
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/temporal"
)

func TestBuild_PlanFinalMembershipIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id int64
	if err = db.QueryRow(ctx, `INSERT INTO run (country,rel,state,snapshot) VALUES ('vn','20990101','running','{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	run := stage.Run{ID: id, Country: "vn", Rel: "20990101", Snapshot: stage.Snapshot{SourceList: []byte(`{"vbpl":{"selection_version":1}}`)}}
	env := stage.Env{DB: db}
	if _, err = New(Config{}).Plan(ctx, env, run); err == nil || !strings.Contains(err.Error(), "final membership") {
		t.Fatalf("missing final: %v", err)
	}
	entries := []membership.Entry{{DocumentKey: "vn:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}}
	if err = membership.PublishPhase(ctx, store.New(db), id, membership.Final, entries); err != nil {
		t.Fatal(err)
	}
	_, err = New(Config{}).Plan(ctx, env, run)
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) || !appErr.NonRetryable() || appErr.Type() != "membership_unreconciled" || !strings.Contains(err.Error(), "vn:vbpl:1") {
		t.Fatalf("unreconciled: %v", err)
	}
}
