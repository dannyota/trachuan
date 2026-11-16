package serve

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (e *Engine) searchGaps(ctx context.Context, tx pgx.Tx, versions []int64, p projection) ([]Gap, error) {
	if len(versions) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT g.kind,'fw:'||v.framework_code||':'||v.version,g.subject,g.detail
 FROM quality_gap g JOIN framework_version v ON v.id=g.version_id
 WHERE g.version_id=ANY($1::bigint[]) AND (g.class='public' OR (g.class='text' AND g.version_id=ANY($2::bigint[])))
 ORDER BY g.kind COLLATE "C",v.framework_code COLLATE "C",v.version COLLATE "C",g.subject COLLATE "C",g.detail COLLATE "C"`, versions, p.readable)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := []Gap{}
	for rows.Next() {
		var g Gap
		if rows.Scan(&g.Kind, &g.Version, &g.Subject, &g.Detail) != nil {
			return nil, ErrUnavailable
		}
		result = append(result, g)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}
