package publicsummary

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type MetadataDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Metadata(db MetadataDB) func(context.Context) (Release, error) {
	return func(ctx context.Context) (Release, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		var r Release
		err := db.QueryRow(ctx, `SELECT active_db, activated_at, CASE WHEN summary_database=active_db THEN summary_projection ELSE 0 END, COALESCE(summary_database=active_db AND headings_approved,false) FROM corpus_release WHERE country='vn'`).Scan(&r.Database, &r.ActivatedAt, &r.Projection, &r.HeadingsApproved)
		if err != nil {
			return Release{}, fmt.Errorf("read public release metadata: %w", err)
		}
		r.ID = strings.TrimPrefix(r.Database, "corpus_vn_")
		return r, nil
	}
}

func (p *postgresPool) Row(ctx context.Context, key string) (Row, error) {
	ctx, done := p.work(ctx)
	defer done()
	conn, err := p.acquire(ctx)
	if err != nil {
		return Row{}, err
	}
	defer p.mu.Unlock()
	var r Row
	err = conn.QueryRow(ctx, `SELECT document_key,indexed,status_class,status_basis,is_current,validity_conflict,eff_from,eff_to,gap_kinds,payload FROM document_summary WHERE projection=1 AND document_key=$1`, key).Scan(&r.Key, &r.Indexed, &r.StatusClass, &r.StatusBasis, &r.IsCurrent, &r.ValidityConflict, &r.EffectiveFrom, &r.EffectiveTo, &r.GapKinds, &r.Payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, ErrNotFound
	}
	if err != nil {
		p.dropConnection()
		return Row{}, fmt.Errorf("query public summary: %w", err)
	}
	return r, nil
}

func (p *postgresPool) List(ctx context.Context) ([]Item, error) {
	ctx, done := p.work(ctx)
	defer done()
	conn, err := p.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer p.mu.Unlock()
	rows, err := conn.Query(ctx, `SELECT document_key,indexed FROM document_summary WHERE projection=1 ORDER BY document_key COLLATE "C" LIMIT 10001`)
	if err != nil {
		p.dropConnection()
		return nil, fmt.Errorf("query public summary list: %w", err)
	}
	defer rows.Close()
	items := make([]Item, 0)
	size := 0
	for rows.Next() {
		var item Item
		if err = rows.Scan(&item.Key, &item.Index); err != nil {
			return nil, fmt.Errorf("scan public summary list: %w", err)
		}
		size += len(item.Key)
		if len(items) == 10000 || size > 1<<20 {
			return nil, ErrTooLarge
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		p.dropConnection()
		return nil, fmt.Errorf("read public summary list: %w", err)
	}
	return items, nil
}
