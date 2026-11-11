package app

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

func (s credentialStore) decideReportIDs(ctx context.Context, p store.DecideReportsParams) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, fmt.Errorf("begin decision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, "SELECT id FROM issue_report WHERE id = ANY($1::bigint[]) FOR UPDATE", p.Ids)
	if err != nil {
		return 0, fmt.Errorf("find reports: %w", err)
	}
	found := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("read reports: %w", err)
		}
		found[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, fmt.Errorf("read reports: %w", err)
	}
	for _, id := range p.Ids {
		if !found[id] {
			return 0, errors.New("unknown report id")
		}
	}
	n, err := s.WithTx(tx).DecideReports(ctx, p)
	if err != nil {
		return 0, fmt.Errorf("update reports: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit decision: %w", err)
	}
	return n, nil
}
