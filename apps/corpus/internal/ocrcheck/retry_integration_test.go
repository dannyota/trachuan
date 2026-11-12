//go:build integration

package ocrcheck

import (
	"context"
	"testing"
	"time"
)

func TestRetryState_DatabaseHistory(t *testing.T) {
	r, _, _, docs := databaseFixture(t)
	ctx := context.Background()
	repo := r.Repository
	checker := options().CheckerID()
	insert := func(checker, state string, usage Usage) {
		t.Helper()
		attempts, err := repo.Begin(ctx, []Intent{{Entry: docs[0].Entry}}, checker, "direct", "0123456789abcdef0123456789abcdef", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if state == "failed" {
			usage.Final = true
			if err := repo.Finish(ctx, attempts[0], docs[0].Entry, usage, nil, "test"); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert(checker, "failed", Usage{CostMicroUSD: 1})
	insert(checker, "failed", Usage{Input: 1})
	insert("other", "failed", Usage{Input: 1})
	insert(checker, "failed", Usage{})
	count, unbilled, err := repo.RetryState(ctx, docs[0].Entry.ID, checker)
	if err != nil || count != 2 || !unbilled {
		t.Fatalf("count=%d unbilled=%v err=%v", count, unbilled, err)
	}
	insert(checker, "preparing", Usage{})
	count, unbilled, err = repo.RetryState(ctx, docs[0].Entry.ID, checker)
	if err != nil || count != 2 || unbilled {
		t.Fatalf("count=%d unbilled=%v err=%v", count, unbilled, err)
	}
	count, unbilled, err = repo.RetryState(ctx, docs[0].Entry.ID+1, checker)
	if err != nil || count != 0 || unbilled {
		t.Fatalf("count=%d unbilled=%v err=%v", count, unbilled, err)
	}
}
