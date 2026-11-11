//go:build integration

package reports

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

type admissionResolver struct{}

func (admissionResolver) Resolve(context.Context, string, string) (Lease, error) {
	return Lease{Corpus: fakeCorpus{}, Release: "20991111", Done: func() {}}, nil
}

func TestPGXRunner_AdmissionDedupeAndCaps(t *testing.T) {
	ctx := t.Context()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	q := store.New(db)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "report-adapter", Email: "report-adapter@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: PGXRunner{DB: db}, Resolver: admissionResolver{}}
	var wg sync.WaitGroup
	results := make(chan Result, 8)
	for range 8 {
		wg.Go(func() {
			result, submitErr := svc.Submit(ctx, account.ID, validInput("wrong_text"))
			if submitErr != nil {
				t.Error(submitErr)
				return
			}
			results <- result
		})
	}
	wg.Wait()
	close(results)
	var id int64
	created := 0
	received := 0
	for result := range results {
		received++
		if id == 0 {
			id = result.ReportID
		}
		if id != result.ReportID || result.State != "open" {
			t.Fatal("dedupe returned different rows")
		}
		if !result.Deduplicated {
			created++
		}
	}
	if created != 1 || received != 8 {
		t.Fatal("duplicate insertion")
	}
	for i := 1; i < 20; i++ {
		in := validInput("missing_document")
		in.Suggestion = suggestion(map[string]string{"document_number": fmt.Sprintf("%d/2025/TT", i)})
		if _, err = svc.Submit(ctx, account.ID, in); err != nil {
			t.Fatal(err)
		}
	}
	in := validInput("missing_document")
	in.Suggestion = suggestion(map[string]string{"document_number": "21/2025/TT"})
	_, err = svc.Submit(ctx, account.ID, in)
	requireCode(t, err, ErrRateLimited)
	if result, repeatErr := svc.Submit(ctx, account.ID, validInput("wrong_text")); repeatErr != nil || !result.Deduplicated || result.ReportID != id {
		t.Fatalf("duplicate at cap: %+v %v", result, repeatErr)
	}
	if _, err = q.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: id, AccountID: account.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Submit(ctx, account.ID, validInput("wrong_text"))
	requireCode(t, err, ErrRateLimited)
	counts, err := q.CountAccountReports(ctx, store.CountAccountReportsParams{AccountID: account.ID, Today: time.Now().Add(-24 * time.Hour)})
	if err != nil || counts.Daily != 20 || counts.Open != 19 || counts.Retained != 20 {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}
	if _, err = db.Exec(ctx, `UPDATE account SET blocked_at=now() WHERE id=$1`, account.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Submit(ctx, account.ID, in)
	requireCode(t, err, ErrUnauthenticated)
}

func TestPGXRunner_RollbackKeepsDatabaseUnchanged(t *testing.T) {
	ctx := t.Context()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	q := store.New(db)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "report-rollback", Email: "report-rollback@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := (PGXRunner{DB: db}).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err = tx.InsertReport(ctx, InsertReportParams{AccountID: account.ID, CreatedAt: time.Now(), Report: Report{GroupKey: GroupKey{Kind: "wrong_text", DocumentKey: testKey}, Release: "20991111"}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	count, err := q.CountRetainedReports(ctx)
	if err != nil || count != 0 {
		t.Fatalf("rolled back rows=%d err=%v", count, err)
	}
}

func TestPGXRunner_RollingWindowUsesServiceClock(t *testing.T) {
	ctx := t.Context()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	account, err := store.New(db).CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "report-window", Email: "report-window@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2035, 6, 1, 0, 0, 0, 0, time.UTC)
	now := start
	svc := Service{Store: PGXRunner{DB: db}, Resolver: admissionResolver{}, Clock: func() time.Time { return now }}
	input := func(n int) Input {
		in := validInput("missing_document")
		in.Suggestion = suggestion(map[string]string{"document_number": fmt.Sprintf("%d/2025/TT", n)})
		return in
	}
	var first Result
	for i := 1; i <= 20; i++ {
		result, submitErr := svc.Submit(ctx, account.ID, input(i))
		if submitErr != nil {
			t.Fatal(submitErr)
		}
		if i == 1 {
			first = result
		}
	}
	var created time.Time
	if err = db.QueryRow(ctx, `SELECT created_at FROM issue_report WHERE id=$1`, first.ReportID).Scan(&created); err != nil || !created.Equal(start) {
		t.Fatalf("created_at=%v want=%v err=%v", created, start, err)
	}
	now = start.Add(24 * time.Hour)
	_, err = svc.Submit(ctx, account.ID, input(21))
	requireCode(t, err, ErrRateLimited)
	now = now.Add(time.Microsecond)
	if _, err = svc.Submit(ctx, account.ID, input(21)); err != nil {
		t.Fatalf("expired window rejected report: %v", err)
	}
}

func TestPGXRunner_DailyRetryUsesOldestOwnReport(t *testing.T) {
	ctx := t.Context()
	url := testpg.NewDatabase(t)
	if err := migrate.Up(ctx, url); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	q := store.New(db)
	account, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "retry-owner", Email: "retry-owner@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := q.CreateAccount(ctx, store.CreateAccountParams{GoogleSub: "retry-other", Email: "retry-other@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC)
	insert := func(accountID int64, key string, at time.Time) int64 {
		t.Helper()
		r, e := q.InsertReport(ctx, store.InsertReportParams{AccountID: accountID, Release: "20991111", DocumentKey: key, Kind: "wrong_text", Suggestion: []byte(`{}`), CreatedAt: at})
		if e != nil {
			t.Fatal(e)
		}
		return r.ID
	}
	insert(account.ID, "outside", now.Add(-25*time.Hour))
	insert(other.ID, "other", now.Add(-24*time.Hour+time.Second))
	oldest := insert(account.ID, "oldest", now.Add(-24*time.Hour+2*time.Minute+500*time.Millisecond))
	if _, err = q.WithdrawOwnReport(ctx, store.WithdrawOwnReportParams{ID: oldest, AccountID: account.ID}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 19; i++ {
		insert(account.ID, fmt.Sprintf("key-%d", i), now.Add(-time.Hour))
	}
	svc := Service{Store: PGXRunner{DB: db}, Resolver: admissionResolver{}, Clock: func() time.Time { return now }}
	_, err = svc.Submit(ctx, account.ID, validInput("wrong_text"))
	delay, ok := RetryDelay(err)
	if !ok || delay != 2*time.Minute+500*time.Millisecond {
		t.Fatalf("retry %v, err %v", delay, err)
	}
}
