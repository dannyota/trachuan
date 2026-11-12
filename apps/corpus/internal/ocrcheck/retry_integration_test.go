//go:build integration

package ocrcheck

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRetryState_DatabaseHistory(t *testing.T) {
	r, _, _, docs := databaseFixture(t)
	ctx := context.Background()
	repo := r.Repository
	checker := options().CheckerID()
	insert := func(checker, state, reason string, usage Usage) {
		t.Helper()
		attempts, err := repo.Begin(ctx, []Intent{{Entry: docs[0].Entry}}, checker, "direct", "0123456789abcdef0123456789abcdef", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if state == "failed" {
			usage.Final = true
			if err := repo.Finish(ctx, attempts[0], docs[0].Entry, usage, nil, reason); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert(checker, "failed", "schema:pages", Usage{CostMicroUSD: 1})
	insert(checker, "failed", "schema:pages", Usage{Input: 1})
	insert("other", "failed", "schema:pages", Usage{Input: 1})
	insert(checker, "failed", "render_failed", Usage{})
	count, unbilled, err := repo.RetryState(ctx, docs[0].Entry.ID, checker)
	if err != nil || count != 2 || !unbilled {
		t.Fatalf("count=%d unbilled=%v err=%v", count, unbilled, err)
	}
	insert(checker, "failed", "expired", Usage{})
	count, unbilled, err = repo.RetryState(ctx, docs[0].Entry.ID, checker)
	if err != nil || count != 2 || unbilled {
		t.Fatalf("expired: count=%d unbilled=%v err=%v", count, unbilled, err)
	}
	insert(checker, "preparing", "", Usage{})
	count, unbilled, err = repo.RetryState(ctx, docs[0].Entry.ID, checker)
	if err != nil || count != 2 || unbilled {
		t.Fatalf("count=%d unbilled=%v err=%v", count, unbilled, err)
	}
	count, unbilled, err = repo.RetryState(ctx, docs[0].Entry.ID+1, checker)
	if err != nil || count != 0 || unbilled {
		t.Fatalf("count=%d unbilled=%v err=%v", count, unbilled, err)
	}
}

func TestCheck_DatabaseLockAcrossStoreDirectories(t *testing.T) {
	first, db, _, docs := databaseFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	first.LoadDocuments = func(ctx context.Context) ([]Document, error) {
		close(entered)
		select {
		case <-release:
			return nil, fmt.Errorf("selection stopped")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	done := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		defer close(done)
		_, err := first.Run(context.Background(), docs, options())
		result <- err
	}()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		<-done
	})
	select {
	case <-entered:
	case <-done:
		t.Fatalf("first runner exited before selection: %v", <-result)
	}
	second, _, client, _ := fixture(t)
	second.Repository = NewDatabase(db, t.TempDir())
	_, err := second.Run(context.Background(), docs, options())
	if err == nil || !strings.Contains(err.Error(), "another OCR check for this checker is running") || client.DirectCalls != 0 {
		t.Fatalf("err=%v calls=%d", err, client.DirectCalls)
	}
	close(release)
	if firstErr := <-result; firstErr == nil {
		t.Fatal("expected first runner selection error")
	}
	_, err = second.Run(context.Background(), docs, options())
	if err != nil || client.DirectCalls != 1 {
		t.Fatalf("err=%v calls=%d", err, client.DirectCalls)
	}
}
