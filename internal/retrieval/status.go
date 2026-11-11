package retrieval

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// CorpusStatus summarizes the evidence stored in one release.
// NewestSourceDate is the latest valid document raw_issue_date, as a civil date.
type CorpusStatus struct {
	Release          string           `json:"release"`
	Documents        int64            `json:"documents"`
	Provisions       int64            `json:"provisions"`
	Nodes            int64            `json:"nodes"`
	Gaps             map[string]int64 `json:"gaps"`
	NewestSourceDate *time.Time       `json:"newest_source_date"`
}

// Status counts the pinned corpus in one consistent read-only snapshot.
func (e *Engine) Status(ctx context.Context) (CorpusStatus, error) {
	result := CorpusStatus{Release: e.release.Rel, Gaps: make(map[string]int64)}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM document),(SELECT count(*) FROM provision),(SELECT count(*) FROM taxonomy_node)`).Scan(&result.Documents, &result.Provisions, &result.Nodes)
	if err != nil {
		return result, databaseFailure("count corpus", err)
	}
	rows, err := tx.Query(ctx, `SELECT kind,count(*) FROM coverage_gap GROUP BY kind`)
	if err != nil {
		return result, databaseFailure("count gaps", err)
	}
	for rows.Next() {
		var kind string
		var count int64
		if err = rows.Scan(&kind, &count); err != nil {
			rows.Close()
			return result, databaseFailure("scan gap count", err)
		}
		result.Gaps[kind] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, databaseFailure("read gap counts", err)
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT raw_issue_date FROM document WHERE raw_issue_date <> ''`)
	if err != nil {
		return result, databaseFailure("query source dates", err)
	}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return result, databaseFailure("scan source date", err)
		}
		date := sourceDate(raw)
		if date != nil && (result.NewestSourceDate == nil || date.After(*result.NewestSourceDate)) {
			result.NewestSourceDate = date
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, databaseFailure("read source dates", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return result, databaseFailure("commit corpus status", err)
	}
	return result, nil
}

func (e *Engine) metadataTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, databaseFailure("begin corpus metadata", err)
	}
	if err = configureMetadataTx(ctx, tx, e.deadline(), reportRollbackTimeout); err != nil {
		return nil, err
	}
	return tx, nil
}

func configureMetadataTx(ctx context.Context, tx pgx.Tx, deadline, rollbackTimeout time.Duration) error {
	if err := setLocal(ctx, tx, "statement_timeout", strconv.FormatInt(deadline.Milliseconds(), 10)); err != nil {
		rollbackReport(ctx, tx, rollbackTimeout)
		return fmt.Errorf("set metadata deadline: %w", err)
	}
	return nil
}

func sourceDate(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.DateOnly, "02/01/2006", "2006-01-02T15:04:05", time.RFC3339} {
		if layout == "2006-01-02T15:04:05" && len(raw) != len(layout) {
			continue
		}
		date, err := time.Parse(layout, raw)
		if err != nil || date.Year() < 1 {
			continue
		}
		if layout == time.RFC3339 {
			date = date.In(time.FixedZone("Vietnam", 7*60*60))
		}
		civil := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
		return &civil
	}
	return nil
}
