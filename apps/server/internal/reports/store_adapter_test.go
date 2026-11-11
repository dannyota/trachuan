package reports

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type adapterRow struct {
	values []any
	err    error
}

func (r adapterRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("wrong scan arity")
	}
	for i, value := range r.values {
		dst := reflect.ValueOf(dest[i]).Elem()
		if value == nil {
			dst.SetZero()
		} else {
			dst.Set(reflect.ValueOf(value))
		}
	}
	return nil
}

type adapterTx struct {
	pgx.Tx
	row                             func(string, []any) pgx.Row
	queries                         []string
	execErr, commitErr, rollbackErr error
	commits, rollbacks              int
	rollbackCanceled                bool
}

func (tx *adapterTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	tx.queries = append(tx.queries, sql)
	return tx.row(sql, args)
}
func (tx *adapterTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.queries = append(tx.queries, sql)
	return pgconn.CommandTag{}, tx.execErr
}
func (tx *adapterTx) Commit(context.Context) error { tx.commits++; return tx.commitErr }
func (tx *adapterTx) Rollback(ctx context.Context) error {
	tx.rollbacks++
	tx.rollbackCanceled = ctx.Err() != nil
	return tx.rollbackErr
}

type adapterBeginner struct {
	tx  *adapterTx
	err error
}

func (b adapterBeginner) Begin(context.Context) (pgx.Tx, error) { return b.tx, b.err }

func TestQueryStore_MapsAdmission(t *testing.T) {
	now := testNow()
	report := Report{GroupKey: GroupKey{DocumentKey: testKey, Anchor: "dieu-1", Kind: "broken_citation", Target: "196-HDBT"}, Release: "20261111", DocumentNumber: "50/2024/TT-NHNN", Suggestion: Suggestion{DocumentNumber: "196-HDBT"}}
	tx := &adapterTx{}
	tx.row = func(sql string, args []any) pgx.Row {
		switch {
		case strings.Contains(sql, "name: LockAccountForReport"):
			if !reflect.DeepEqual(args, []any{int64(7)}) {
				t.Fatal(args)
			}
			return adapterRow{values: []any{&now, &now}}
		case strings.Contains(sql, "name: CountAccountReports"):
			if !reflect.DeepEqual(args, []any{now, int64(7)}) {
				t.Fatal(args)
			}
			return adapterRow{values: []any{int64(4), int64(2), int64(6)}}
		case strings.Contains(sql, "name: CountRetainedReports"):
			return adapterRow{values: []any{int64(9)}}
		case strings.Contains(sql, "name: FindOpenReport"):
			if !reflect.DeepEqual(args, []any{int64(7), testKey, "dieu-1", "broken_citation", "196-HDBT"}) {
				t.Fatal(args)
			}
		case strings.Contains(sql, "name: InsertReport"):
			if len(args) != 9 || args[8] != now {
				t.Fatalf("insert did not receive admission time: %v", args)
			}
			if !reflect.DeepEqual(args[:7], []any{int64(7), "20261111", testKey, "dieu-1", "50/2024/TT-NHNN", "broken_citation", "196-HDBT"}) {
				t.Fatal(args)
			}
			var got Suggestion
			if err := json.Unmarshal(args[7].(json.RawMessage), &got); err != nil || got != report.Suggestion {
				t.Fatalf("suggestion=%+v err=%v", got, err)
			}
		default:
			t.Fatal("unexpected query")
		}
		return adapterRow{values: []any{int64(42), int64(7), report.Release, testKey, report.Anchor, report.DocumentNumber, report.Kind, report.Target, json.RawMessage(`{}`), "open", now, now, nil, nil}}
	}
	q := QueryStore{Queries: store.New(tx)}
	account, err := q.LockAccountForReport(t.Context(), LockAccountForReportParams{AccountID: 7})
	if err != nil || account != (AccountState{Exists: true, Blocked: true, DeletionRequested: true}) {
		t.Fatalf("%+v %v", account, err)
	}
	counts, err := q.CountAccountReports(t.Context(), CountAccountReportsParams{AccountID: 7, Since: now})
	if err != nil || counts != (AccountCounts{Daily: 4, Open: 2, Retained: 6}) {
		t.Fatalf("%+v %v", counts, err)
	}
	retained, err := q.CountRetainedReports(t.Context())
	if err != nil || retained != 9 {
		t.Fatalf("%d %v", retained, err)
	}
	if err = q.LockReports(t.Context()); err != nil || !strings.Contains(tx.queries[len(tx.queries)-1], "pg_advisory_xact_lock") {
		t.Fatal("missing admission lock", err)
	}
	result, found, err := q.FindOpenReport(t.Context(), FindOpenReportParams{AccountID: 7, GroupKey: report.GroupKey})
	if err != nil || !found || result.ReportID != 42 || result.State != "open" {
		t.Fatalf("%+v %v %v", result, found, err)
	}
	result, err = q.InsertReport(t.Context(), InsertReportParams{AccountID: 7, Report: report, CreatedAt: now})
	if err != nil || result.ReportID != 42 || result.State != "open" || result.Deduplicated {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestQueryStore_MissingRowsAndErrors(t *testing.T) {
	for _, cause := range []error{pgx.ErrNoRows, errors.New("PRIVATE")} {
		tx := &adapterTx{row: func(string, []any) pgx.Row { return adapterRow{err: cause} }}
		q := QueryStore{Queries: store.New(tx)}
		account, err := q.LockAccountForReport(t.Context(), LockAccountForReportParams{})
		if cause == pgx.ErrNoRows {
			if err != nil || account.Exists {
				t.Fatal(account, err)
			}
		} else if !errors.Is(err, cause) {
			t.Fatal(err)
		}
		_, found, err := q.FindOpenReport(t.Context(), FindOpenReportParams{})
		if cause == pgx.ErrNoRows {
			if err != nil || found {
				t.Fatal(found, err)
			}
		} else if !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if _, err = q.CountAccountReports(t.Context(), CountAccountReportsParams{}); !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if _, err = q.CountRetainedReports(t.Context()); !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if _, err = q.InsertReport(t.Context(), InsertReportParams{}); !errors.Is(err, cause) {
			t.Fatal(err)
		}
	}
}

func TestPGXRunner_BindsTransaction(t *testing.T) {
	cause := errors.New("PRIVATE")
	tx := &adapterTx{commitErr: cause, rollbackErr: cause}
	runner := PGXRunner{DB: adapterBeginner{tx: tx}}
	bound, err := runner.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = bound.LockReports(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(tx.queries) != 2 || !strings.Contains(tx.queries[0], "statement_timeout") {
		t.Fatal(tx.queries)
	}
	if err = bound.Commit(t.Context()); !errors.Is(err, cause) || tx.commits != 1 {
		t.Fatal(err)
	}
	if err = bound.Rollback(t.Context()); !errors.Is(err, cause) || tx.rollbacks != 1 {
		t.Fatal(err)
	}
	runner.DB = adapterBeginner{err: cause}
	if _, err = runner.Begin(t.Context()); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	tx = &adapterTx{execErr: cause}
	runner.DB = adapterBeginner{tx: tx}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = runner.Begin(ctx); !errors.Is(err, cause) || tx.rollbacks != 1 || tx.rollbackCanceled {
		t.Fatal("failed setup leaked transaction", err)
	}
}
