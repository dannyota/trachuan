//go:build integration

package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestStore_ExportRaceWithReportRetention(t *testing.T) {
	ctx, conn, q := setup(t)
	account := webAccount(t, ctx, conn, "export-race")
	r := report(t, ctx, q, account, "vn:export-race")
	if _, err := q.DecideReports(ctx, store.DecideReportsParams{Decision: "accepted", Ids: []int64{r.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE issue_report SET changed_at = now() - interval '100 days' WHERE id = $1`, r.ID); err != nil {
		t.Fatal(err)
	}
	other, err := pgx.ConnectConfig(ctx, conn.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close(ctx) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = store.New(tx).CreateExportBatch(ctx, "race-batch"); err != nil {
		t.Fatal(err)
	}
	if _, err = other.Exec(ctx, `SET lock_timeout = '200ms'`); err != nil {
		t.Fatal(err)
	}
	n, err := store.New(other).DeleteExpiredReports(ctx, store.DeleteExpiredReportsParams{Before: time.Now().Add(-90 * 24 * time.Hour), BatchLimit: 10})
	if err != nil || n != 0 {
		t.Fatalf("purge during export: deleted=%d err=%v", n, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	n, err = q.DeleteExpiredReports(ctx, store.DeleteExpiredReportsParams{Before: time.Now().Add(-90 * 24 * time.Hour), BatchLimit: 10})
	if err != nil || n != 0 {
		t.Fatalf("purge after export: deleted=%d err=%v", n, err)
	}
	var exists bool
	if err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM issue_report WHERE id = $1 AND export_batch = 'race-batch')`, r.ID).Scan(&exists); err != nil || !exists {
		t.Fatalf("exported report exists=%v err=%v", exists, err)
	}
}

func TestStore_ExpiredPartialBatchIsNotReadable(t *testing.T) {
	ctx, conn, q := setup(t)
	account := webAccount(t, ctx, conn, "expired-batch")
	for _, key := range []string{"vn:a", "vn:b", "vn:c"} {
		r := report(t, ctx, q, account, key)
		if _, err := q.DecideReports(ctx, store.DecideReportsParams{Decision: "accepted", Ids: []int64{r.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := q.CreateExportBatch(ctx, "old-batch"); err != nil || n != 3 {
		t.Fatalf("export count=%d err=%v", n, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE issue_report_export SET created_at = now() - interval '100 days' WHERE batch_id = 'old-batch'`); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-90 * 24 * time.Hour)
	n, err := q.DeleteExpiredSnapshots(ctx, store.DeleteExpiredSnapshotsParams{Before: before, BatchLimit: 1})
	if err != nil || n != 1 {
		t.Fatalf("purge count=%d err=%v", n, err)
	}
	snapshot, err := q.BatchSnapshot(ctx, store.BatchSnapshotParams{BatchID: "old-batch", Before: before})
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ReportID    int64  `json:"report_id"`
		DocumentKey string `json:"document_key"`
	}
	if err = json.Unmarshal(snapshot.Reports, &rows); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Expired || len(rows) != 0 {
		t.Fatalf("expired=%v count=%d", snapshot.Expired, len(rows))
	}
}

func TestStore_CountAccountReportsCalendarDay(t *testing.T) {
	ctx, conn, q := setup(t)
	owner := webAccount(t, ctx, conn, "count-owner")
	other := webAccount(t, ctx, conn, "count-other")
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 11, 11, 0, 0, 0, 0, loc)
	old := report(t, ctx, q, owner, "vn:old")
	open := report(t, ctx, q, owner, "vn:open")
	closed := report(t, ctx, q, owner, "vn:closed")
	otherReport := report(t, ctx, q, other, "vn:other")
	if _, err = conn.Exec(ctx, `UPDATE issue_report SET created_at = CASE WHEN id = $1 THEN $2::timestamptz - interval '1 microsecond' ELSE $2::timestamptz END WHERE id = ANY($3::bigint[])`, old.ID, cutoff, []int64{old.ID, open.ID, closed.ID, otherReport.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = q.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: closed.ID, AccountID: owner}); err != nil {
		t.Fatal(err)
	}
	counts, err := q.CountAccountReports(ctx, store.CountAccountReportsParams{AccountID: owner, Today: cutoff})
	if err != nil || counts.Daily != 2 || counts.Open != 2 || counts.Retained != 3 {
		t.Fatalf("owner counts=%+v err=%v", counts, err)
	}
	counts, err = q.CountAccountReports(ctx, store.CountAccountReportsParams{AccountID: other, Today: cutoff})
	if err != nil || counts.Daily != 1 || counts.Open != 1 || counts.Retained != 1 {
		t.Fatalf("other counts=%+v err=%v", counts, err)
	}
}
