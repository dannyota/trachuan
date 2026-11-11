package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

// Beginner opens a pgx transaction on the auth database.
type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// PGXRunner binds admission queries and completion to one pgx transaction.
type PGXRunner struct{ DB Beginner }

// Begin opens an admission transaction with the auth statement deadline.
func (r PGXRunner) Begin(ctx context.Context) (Transaction, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin report admission: %w", err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout = '2s'`); err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), authDeadline)
		defer cancel()
		_ = tx.Rollback(cleanup)
		return nil, fmt.Errorf("set report admission deadline: %w", err)
	}
	return &pgxTransaction{QueryStore: QueryStore{Queries: store.New(tx)}, tx: tx}, nil
}

type pgxTransaction struct {
	QueryStore
	tx pgx.Tx
}

func (t *pgxTransaction) Commit(ctx context.Context) error {
	if err := t.tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit report admission: %w", err)
	}
	return nil
}

func (t *pgxTransaction) Rollback(ctx context.Context) error {
	if err := t.tx.Rollback(ctx); err != nil {
		return fmt.Errorf("rollback report admission: %w", err)
	}
	return nil
}

// QueryStore adapts generated auth queries to the report admission interface.
// Admission must use the instance PGXRunner binds to its transaction.
type QueryStore struct{ Queries *store.Queries }

// LockAccountForReport locks the account and preserves its admission flags.
func (s QueryStore) LockAccountForReport(ctx context.Context, p LockAccountForReportParams) (AccountState, error) {
	row, err := s.Queries.LockAccountForReport(ctx, p.AccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountState{}, nil
	}
	if err != nil {
		return AccountState{}, fmt.Errorf("lock report account: %w", err)
	}
	return AccountState{Exists: true, Blocked: row.BlockedAt != nil, DeletionRequested: row.DeleteRequestedAt != nil}, nil
}

// LockReports serializes admission against all report and snapshot counts.
func (s QueryStore) LockReports(ctx context.Context) error {
	if err := s.Queries.LockReports(ctx); err != nil {
		return fmt.Errorf("lock reports: %w", err)
	}
	return nil
}

// CountAccountReports counts retained rows and the rolling admission window.
func (s QueryStore) CountAccountReports(ctx context.Context, p CountAccountReportsParams) (AccountCounts, error) {
	row, err := s.Queries.CountAccountReports(ctx, store.CountAccountReportsParams{Today: p.Since, AccountID: p.AccountID})
	if err != nil {
		return AccountCounts{}, fmt.Errorf("count account reports: %w", err)
	}
	return AccountCounts{Daily: row.Daily, Open: row.Open, Retained: row.Retained}, nil
}

// CountRetainedReports includes both report rows and export snapshot rows.
func (s QueryStore) CountRetainedReports(ctx context.Context) (int64, error) {
	count, err := s.Queries.CountRetainedReports(ctx)
	if err != nil {
		return 0, fmt.Errorf("count retained reports: %w", err)
	}
	return count, nil
}

// FindOpenReport returns only the caller's exact open group, or found false.
func (s QueryStore) FindOpenReport(ctx context.Context, p FindOpenReportParams) (Result, bool, error) {
	row, err := s.Queries.FindOpenReport(ctx, store.FindOpenReportParams{AccountID: p.AccountID, DocumentKey: p.DocumentKey, Anchor: p.Anchor, Kind: p.Kind, Target: p.Target})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("find open report: %w", err)
	}
	return Result{ReportID: row.ID, State: row.State}, true, nil
}

// InsertReport stores the canonical report fields and admission time.
func (s QueryStore) InsertReport(ctx context.Context, p InsertReportParams) (Result, error) {
	suggestion, err := json.Marshal(p.Report.Suggestion)
	if err != nil {
		return Result{}, fmt.Errorf("encode report suggestion: %w", err)
	}
	r := p.Report
	row, err := s.Queries.InsertReport(ctx, store.InsertReportParams{AccountID: p.AccountID, Release: r.Release, DocumentKey: r.DocumentKey, Anchor: r.Anchor, DocumentNumber: r.DocumentNumber, Kind: r.Kind, Target: r.Target, Suggestion: suggestion, CreatedAt: p.CreatedAt})
	if err != nil {
		return Result{}, fmt.Errorf("insert report: %w", err)
	}
	return Result{ReportID: row.ID, State: row.State}, nil
}
