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

func report(t *testing.T, ctx context.Context, q *store.Queries, account int64, key string) store.IssueReport {
	t.Helper()
	r, err := q.InsertReport(ctx, store.InsertReportParams{AccountID: account, Release: "release", DocumentKey: key, Kind: "wrong_text", Suggestion: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStore_ReportOwnRowsOnly(t *testing.T) {
	ctx, conn, q := setup(t)
	owner := webAccount(t, ctx, conn, "report-owner")
	other := webAccount(t, ctx, conn, "report-other")
	r := report(t, ctx, q, other, "vn:other")
	rows, err := q.ListOwnReports(ctx, store.ListOwnReportsParams{AccountID: owner, PageSize: 100})
	if err != nil || len(rows) != 0 {
		t.Fatalf("own list count=%d err=%v", len(rows), err)
	}
	n, err := q.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: r.ID, AccountID: owner})
	if err != nil || n != 0 {
		t.Fatalf("other withdrawal: %d %v", n, err)
	}
	n, err = q.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: r.ID, AccountID: other})
	if err != nil || n != 1 {
		t.Fatalf("own withdrawal: %d %v", n, err)
	}
}

func TestStore_ExportBatchSnapshotSurvivesDeletion(t *testing.T) {
	ctx, conn, q := setup(t)
	account := webAccount(t, ctx, conn, "report-export")
	r := report(t, ctx, q, account, "vn:export")
	by := "operator"
	if n, err := q.DecideReports(ctx, store.DecideReportsParams{Decision: "accepted", DecidedBy: &by, Ids: []int64{r.ID}}); err != nil || n != 1 {
		t.Fatalf("decide: %d %v", n, err)
	}
	if n, err := q.CreateExportBatch(ctx, "batch"); err != nil || n != 1 {
		t.Fatalf("export: %d %v", n, err)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM account WHERE id = $1`, account); err != nil {
		t.Fatal(err)
	}
	rows, err := q.BatchSnapshot(ctx, "batch")
	if err != nil || len(rows) != 1 || rows[0].ReportID != r.ID || rows[0].DocumentKey != r.DocumentKey {
		t.Fatalf("snapshot count=%d err=%v", len(rows), err)
	}
}

func TestStore_SnapshotsCountTowardRetainedCap(t *testing.T) {
	ctx, conn, q := setup(t)
	account := webAccount(t, ctx, conn, "report-cap")
	r := report(t, ctx, q, account, "vn:cap")
	if _, err := q.DecideReports(ctx, store.DecideReportsParams{Decision: "accepted", Ids: []int64{r.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateExportBatch(ctx, "batch"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM issue_report WHERE id = $1`, r.ID); err != nil {
		t.Fatal(err)
	}
	report(t, ctx, q, account, "vn:next")
	n, err := q.CountRetainedReports(ctx)
	if err != nil || n != 2 {
		t.Fatalf("retained: %d %v", n, err)
	}
}

func TestStore_RetentionDeletesClosed(t *testing.T) {
	ctx, conn, q := setup(t)
	account := webAccount(t, ctx, conn, "report-retention")
	closed := report(t, ctx, q, account, "vn:closed")
	report(t, ctx, q, account, "vn:open")
	if _, err := q.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: closed.ID, AccountID: account}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE issue_report SET changed_at = now() - interval '100 days'`); err != nil {
		t.Fatal(err)
	}
	n, err := q.DeleteExpiredReports(ctx, store.DeleteExpiredReportsParams{Before: time.Now().Add(-90 * 24 * time.Hour), BatchLimit: 10})
	if err != nil || n != 1 {
		t.Fatalf("deleted: %d %v", n, err)
	}
	rows, err := q.ListOwnReports(ctx, store.ListOwnReportsParams{AccountID: account, PageSize: 100})
	if err != nil || len(rows) != 1 || rows[0].State != "open" {
		t.Fatalf("remaining count=%d err=%v", len(rows), err)
	}
}

func TestStore_DedupeAndCapsUnderRace(t *testing.T) {
	ctx, conn, _ := setup(t)
	account := webAccount(t, ctx, conn, "report-race")
	other, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close(ctx) })
	submit := func(c *pgx.Conn, key string, cap int64) error {
		tx, err := c.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		q := store.New(tx)
		if _, err = q.LockAccountForReport(ctx, account); err != nil {
			return err
		}
		if err = q.LockReports(ctx); err != nil {
			return err
		}
		_, err = q.FindOpenReport(ctx, store.FindOpenReportParams{AccountID: account, DocumentKey: key, Kind: "wrong_text"})
		if err == nil {
			return tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		counts, err := q.CountAccountReports(ctx, store.CountAccountReportsParams{AccountID: account, Today: time.Now().Add(-24 * time.Hour)})
		if err != nil {
			return err
		}
		if counts.Retained >= cap {
			return tx.Commit(ctx)
		}
		_, err = q.InsertReport(ctx, store.InsertReportParams{AccountID: account, Release: "release", DocumentKey: key, Kind: "wrong_text", Suggestion: []byte(`{}`)})
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	for _, keys := range [][2]string{{"vn:same", "vn:same"}, {"vn:first", "vn:second"}} {
		errs := make(chan error, 2)
		start := make(chan struct{})
		for i, c := range []*pgx.Conn{conn, other} {
			go func(c *pgx.Conn, key string) { <-start; errs <- submit(c, key, 2) }(c, keys[i])
		}
		close(start)
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM issue_report WHERE account_id = $1`, account).Scan(&count); err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestStore_RetainedCapAcrossWithdrawals(t *testing.T) {
	ctx, conn, q := setup(t)
	accounts := []int64{webAccount(t, ctx, conn, "cycle-one"), webAccount(t, ctx, conn, "cycle-two")}
	other, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close(ctx) })
	submit := func(c *pgx.Conn, account int64, key string) error {
		tx, err := c.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		queries := store.New(tx)
		if _, err = queries.LockAccountForReport(ctx, account); err != nil {
			return err
		}
		if err = queries.LockReports(ctx); err != nil {
			return err
		}
		count, err := queries.CountRetainedReports(ctx)
		if err != nil {
			return err
		}
		if count >= 3 {
			return tx.Commit(ctx)
		}
		r, err := queries.InsertReport(ctx, store.InsertReportParams{AccountID: account, Release: "release", DocumentKey: key, Kind: "wrong_text", Suggestion: []byte(`{}`)})
		if err != nil {
			return err
		}
		if _, err = queries.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: r.ID, AccountID: account}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	for range 3 {
		start := make(chan struct{})
		results := make(chan error, 2)
		for i, c := range []*pgx.Conn{conn, other} {
			go func(c *pgx.Conn, account int64) { <-start; results <- submit(c, account, "vn:cycle") }(c, accounts[i])
		}
		close(start)
		for range 2 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
	}
	count, err := q.CountRetainedReports(ctx)
	if err != nil || count != 3 {
		t.Fatalf("retained=%d err=%v", count, err)
	}
}
