package retrieval

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

const reportRollbackTimeout = 2 * time.Second

const (
	reportSegmentLimit = 50_000
	reportSegmentBytes = 32
)

type reportRollbacker interface {
	Rollback(context.Context) error
}

func rollbackReport(ctx context.Context, tx reportRollbacker, timeout time.Duration) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	_ = tx.Rollback(cleanup)
}

// LookupDocument returns the original number and existence of a document key.
func (e *Engine) LookupDocument(ctx context.Context, key string) (Document, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return Document{}, false, err
	}
	defer rollbackReport(ctx, tx, reportRollbackTimeout)
	var doc Document
	err = tx.QueryRow(ctx, `SELECT number FROM document WHERE key=$1`, key).Scan(&doc.Number)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, databaseFailure("lookup report document", err)
	}
	return doc, true, nil
}

// AnchorExists checks an exact anchor within one document, including unchunked nodes.
func (e *Engine) AnchorExists(ctx context.Context, key, anchor string) (bool, error) {
	return e.reportExists(ctx, `SELECT EXISTS(SELECT 1 FROM provision p JOIN document d ON d.id=p.document_id WHERE d.key=$1 AND p.anchor=$2)`, key, anchor)
}

// HasNumber tests normalized-number membership without requiring uniqueness.
func (e *Engine) HasNumber(ctx context.Context, number string) (bool, error) {
	return e.reportExists(ctx, `SELECT EXISTS(SELECT 1 FROM document WHERE number_norm=$1)`, number)
}

func (e *Engine) reportExists(ctx context.Context, query string, args ...any) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return false, err
	}
	defer rollbackReport(ctx, tx, reportRollbackTimeout)
	var exists bool
	if err = tx.QueryRow(ctx, query, args...).Scan(&exists); err != nil {
		return false, databaseFailure("lookup report identity", err)
	}
	return exists, nil
}

// HasNumberSegment tests membership in the immutable set loaded for this release.
func (e *Engine) HasNumberSegment(ctx context.Context, segment string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, databaseFailure("read report number segments", err)
	}
	_, ok := e.numberSegments[segment]
	return ok, nil
}

func (e *Engine) loadNumberSegments(ctx context.Context) (map[string]struct{}, error) {
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, err
	}
	defer rollbackReport(ctx, tx, reportRollbackTimeout)
	rows, err := tx.Query(ctx, `SELECT DISTINCT segment COLLATE "C" AS segment FROM document CROSS JOIN LATERAL regexp_split_to_table(number_norm, '[/-]') AS segment WHERE segment <> '' AND octet_length(segment) <= $1 ORDER BY segment LIMIT $2`, reportSegmentBytes, reportSegmentLimit)
	if err != nil {
		return nil, databaseFailure("query report number segments", err)
	}
	defer rows.Close()
	segments := make(map[string]struct{})
	limitReached := func() { slog.Warn("report number segment limit reached", "release", e.release.Rel) }
	for rows.Next() {
		var segment string
		if err = rows.Scan(&segment); err != nil {
			return nil, databaseFailure("scan report number segment", err)
		}
		addReportSegment(segments, segment, limitReached)
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read report number segments", err)
	}
	return segments, nil
}

func addReportSegment(segments map[string]struct{}, segment string, limitReached func()) {
	if segment == "" || len(segment) > reportSegmentBytes || len(segments) >= reportSegmentLimit {
		return
	}
	segments[segment] = struct{}{}
	if len(segments) == reportSegmentLimit {
		limitReached()
	}
}
