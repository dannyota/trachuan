package serve

import (
	"context"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

func readPostings(ctx context.Context, tx pgx.Tx, versions []int64, indices map[int64]int, public bool) (map[int64]map[string]int, error) {
	sql := `SELECT chunk_id,token,tf FROM body_posting WHERE version_id=ANY($1::bigint[])`
	if public {
		sql = `SELECT chunk_id,token,tf FROM public_posting WHERE version_id=ANY($1::bigint[])`
	}
	rows, err := tx.Query(ctx, sql, versions)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := map[int64]map[string]int{}
	for rows.Next() {
		var id int64
		var token string
		var tf int
		if rows.Scan(&id, &token, &tf) != nil || tf <= 0 {
			return nil, ErrUnavailable
		}
		if _, ok := indices[id]; !ok {
			return nil, ErrUnavailable
		}
		if result[id] == nil {
			result[id] = map[string]int{}
		}
		result[id][token] = tf
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}

func readPublicStats(ctx context.Context, tx pgx.Tx) (lexical.Stats, error) {
	s := lexical.Stats{DF: map[string]int64{}}
	if tx.QueryRow(ctx, `SELECT chunk_count,avg_length FROM public_lexical_stats`).Scan(&s.N, &s.AvgDL) != nil {
		return s, ErrUnavailable
	}
	if s.N == 0 || s.AvgDL == 0 {
		s.AvgDL = 1
	}
	rows, err := tx.Query(ctx, `SELECT token,df FROM public_lexical_df ORDER BY token`)
	if err != nil {
		return s, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var token string
		var df int64
		if rows.Scan(&token, &df) != nil {
			return s, ErrUnavailable
		}
		s.DF[token] = df
	}
	if rows.Err() != nil {
		return s, ErrUnavailable
	}
	return s, nil
}
