//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestFrameworkEntitlement_OverlappingAllowAndRevoke(t *testing.T) {
	for _, tc := range []struct {
		name        string
		revokeFirst bool
		wantErr     error
		wantEvents  []string
	}{
		{name: "revoke cannot see pending allow", wantErr: store.ErrFrameworkNotAllowed, wantEvents: []string{"allow:allower"}},
		{name: "allow waits for pending revoke", revokeFirst: true, wantEvents: []string{"allow:seed", "revoke:revoker", "allow:allower"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, conn, q := setup(t)
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			peer, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = peer.Close(context.Background()) }()
			p := store.FrameworkAllowance{Email: "owner@example.test", Scope: "fw-alpha", By: "allower"}
			r := store.FrameworkRevocation{Email: p.Email, Scope: p.Scope, By: "revoker"}
			if tc.revokeFirst {
				seed := p
				seed.By = "seed"
				if _, err = q.AllowFrameworkEntitlement(ctx, seed); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			var competingErr error
			if tc.revokeFirst {
				if err = q.WithTx(tx).RevokeFrameworkEntitlement(ctx, r); err != nil {
					t.Fatal(err)
				}
				result := make(chan error, 1)
				go func() { _, allowErr := store.New(peer).AllowFrameworkEntitlement(ctx, p); result <- allowErr }()
				if err = waitFrameworkMutationBlocked(ctx, tx, conn.PgConn().PID(), peer.PgConn().PID()); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				competingErr = <-result
			} else {
				if _, err = q.WithTx(tx).AllowFrameworkEntitlement(ctx, p); err != nil {
					t.Fatal(err)
				}
				competingErr = store.New(peer).RevokeFrameworkEntitlement(ctx, r)
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if !errors.Is(competingErr, tc.wantErr) {
				t.Fatalf("competing mutation error=%v want=%v", competingErr, tc.wantErr)
			}
			live, err := q.ListLiveFrameworkEntitlementsByEmail(ctx, p.Email)
			if err != nil || len(live) != 1 || live[0].Scope != "fw-alpha" || live[0].GrantedBy != "allower" {
				t.Fatalf("live=%v error=%v", live, err)
			}
			rows, err := conn.Query(ctx, `SELECT action||':'||actor FROM framework_entitlement_event ORDER BY id`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var events []string
			for rows.Next() {
				var event string
				if err = rows.Scan(&event); err != nil {
					t.Fatal(err)
				}
				events = append(events, event)
			}
			if err = rows.Err(); err != nil || !reflect.DeepEqual(events, tc.wantEvents) {
				t.Fatalf("events=%v want=%v error=%v", events, tc.wantEvents, err)
			}
		})
	}
}

func waitFrameworkMutationBlocked(ctx context.Context, tx pgx.Tx, blocker, waiter uint32) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT $1::integer = ANY(pg_blocking_pids($2::integer))`, blocker, waiter).Scan(&blocked); err != nil {
			return fmt.Errorf("observe framework mutation lock: %w", err)
		}
		if blocked {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for framework mutation lock: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
