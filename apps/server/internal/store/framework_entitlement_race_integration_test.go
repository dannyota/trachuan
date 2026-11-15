//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestFrameworkEntitlement_ConcurrentMutationsHaveOneEvent(t *testing.T) {
	ctx, conn, q := setup(t)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	peer, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = peer.Close(context.Background()) }()
	for _, action := range []string{"allow", "revoke"} {
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, queries := range []*store.Queries{q, store.New(peer)} {
			go func() {
				<-start
				var mutationErr error
				if action == "allow" {
					_, mutationErr = queries.AllowFrameworkEntitlement(ctx, store.FrameworkAllowance{Email: "owner@example.test", Scope: "fw-alpha", By: "operator"})
				} else {
					mutationErr = queries.RevokeFrameworkEntitlement(ctx, store.FrameworkRevocation{Email: "owner@example.test", Scope: "fw-alpha", By: "operator"})
				}
				results <- mutationErr
			}()
		}
		close(start)
		successes, conflicts := 0, 0
		for range 2 {
			mutationErr := <-results
			switch {
			case mutationErr == nil:
				successes++
			case action == "allow" && errors.Is(mutationErr, store.ErrFrameworkAlreadyAllowed), action == "revoke" && errors.Is(mutationErr, store.ErrFrameworkNotAllowed):
				conflicts++
			default:
				t.Fatalf("%s error=%v", action, mutationErr)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("%s successes=%d conflicts=%d", action, successes, conflicts)
		}
		var events int
		if err = conn.QueryRow(ctx, `SELECT count(*) FROM framework_entitlement_event WHERE action=$1`, action).Scan(&events); err != nil || events != 1 {
			t.Fatalf("%s events=%d error=%v", action, events, err)
		}
	}
}
