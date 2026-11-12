package retrieval

import (
	"context"
	"strconv"
	"strings"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

func sparseVector(v lexical.Vector, dimensions int) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(int64(x.Index), 10))
		b.WriteByte(':')
		b.WriteString(strconv.FormatFloat(float64(x.Value), 'g', -1, 32))
	}
	b.WriteString("}/")
	b.WriteString(strconv.Itoa(dimensions))
	return b.String()
}
func lexicalQuery(ctx context.Context, tx pgx.Tx, args ...any) ([]Candidate, error) {
	return lexicalQueryWithValidity(ctx, tx, false, args...)
}

func lexicalQueryWithValidity(ctx context.Context, tx pgx.Tx, notYet bool, args ...any) ([]Candidate, error) {
	sql := `SELECT c.id, -(c.sparse <#> $1::sparsevec) AS score FROM chunk c JOIN document d ON d.id=c.document_id WHERE ` + eligibility + contextScope + ` AND -(c.sparse <#> $1::sparsevec) > 0 ORDER BY score DESC,c.id LIMIT $5`
	sql = validitySQL(sql, notYet)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, databaseFailure("lexical query failed", err)
	}
	defer rows.Close()
	var result []Candidate
	for rows.Next() {
		var c Candidate
		if err = rows.Scan(&c.ChunkID, &c.BM25); err != nil {
			return nil, databaseFailure("read lexical candidate failed", err)
		}
		c.Rank = len(result) + 1
		result = append(result, c)
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read lexical candidates failed", err)
	}
	return result, nil
}
