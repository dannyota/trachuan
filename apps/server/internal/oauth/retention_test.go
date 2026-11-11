package oauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

type retentionDB struct {
	store.DBTX
	t         *testing.T
	remaining map[string]int64
	calls     map[string]int
	after     func()
	fail      string
}

func (db *retentionDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 2*time.Second {
		db.t.Fatal("cleanup query lacks a two-second deadline")
	}
	if args[2] != int32(1000) {
		db.t.Fatalf("batch limit = %v, want 1000", args[2])
	}
	name := strings.Fields(sql)[2]
	db.calls[name]++
	if db.calls[name] > 50 {
		db.t.Fatalf("%s exceeded 50 batches", name)
	}
	if db.after != nil {
		db.after()
	}
	if name == db.fail {
		return pgconn.CommandTag{}, errors.New("cleanup failed")
	}
	n := db.remaining[name]
	if n < 0 {
		n = 1000
	} else {
		n = min(n, 1000)
		db.remaining[name] -= n
	}
	return pgconn.NewCommandTag(fmt.Sprintf("DELETE %d", n)), nil
}

func newRetentionDB(t *testing.T) *retentionDB {
	t.Helper()
	return &retentionDB{t: t, remaining: map[string]int64{}, calls: map[string]int{}}
}

func TestRetention_DrainsBacklogInOnePass(t *testing.T) {
	db := newRetentionDB(t)
	for _, step := range []string{"DeleteExpiredOAuthRequests", "DeleteExpiredOAuthCodes", "DeleteExpiredOAuthTokens", "DeleteRetiredOAuthGrants"} {
		db.remaining[step] = 2501
	}
	retentionPass(t.Context(), store.New(db), time.Now(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	for step, remaining := range db.remaining {
		if remaining != 0 || db.calls[step] != 3 {
			t.Errorf("%s: remaining = %d, calls = %d; want 0, 3", step, remaining, db.calls[step])
		}
	}
}

func TestRetention_CapsEachStepPerPass(t *testing.T) {
	db := newRetentionDB(t)
	for _, step := range []string{"DeleteExpiredOAuthRequests", "DeleteExpiredOAuthCodes", "DeleteExpiredOAuthTokens", "DeleteRetiredOAuthGrants"} {
		db.remaining[step] = -1
	}
	retentionPass(t.Context(), store.New(db), time.Now(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	for step := range db.remaining {
		if db.calls[step] != 50 {
			t.Errorf("%s: calls = %d, want 50", step, db.calls[step])
		}
	}
}

func TestRetention_CancellationStopsBetweenBatches(t *testing.T) {
	db := newRetentionDB(t)
	db.remaining["DeleteExpiredOAuthRequests"] = 2501
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	db.after = func() {
		if db.calls["DeleteExpiredOAuthRequests"] == 2 {
			cancel()
		}
	}
	retentionPass(ctx, store.New(db), time.Now(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if db.calls["DeleteExpiredOAuthRequests"] != 2 || len(db.calls) != 1 || db.remaining["DeleteExpiredOAuthRequests"] != 501 {
		t.Fatalf("cleanup continued after cancellation: calls = %v, remaining = %v", db.calls, db.remaining)
	}
}

func TestRetention_ErrorStopsStepAndContinuesOthers(t *testing.T) {
	db := newRetentionDB(t)
	db.fail = "DeleteExpiredOAuthRequests"
	db.remaining[db.fail] = 2501
	db.remaining["DeleteExpiredOAuthCodes"] = 2501
	retentionPass(t.Context(), store.New(db), time.Now(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if db.calls[db.fail] != 1 || db.remaining[db.fail] != 2501 {
		t.Fatalf("failed step retried: calls = %v, remaining = %v", db.calls, db.remaining)
	}
	if db.remaining["DeleteExpiredOAuthCodes"] != 0 || db.calls["DeleteExpiredOAuthTokens"] != 1 || db.calls["DeleteRetiredOAuthGrants"] != 1 {
		t.Fatalf("later steps did not finish: calls = %v, remaining = %v", db.calls, db.remaining)
	}
}
