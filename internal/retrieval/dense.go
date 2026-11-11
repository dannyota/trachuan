package retrieval

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const eligibility = `(($3::boolean OR (d.is_current AND NOT d.validity_conflict)) AND (d.eff_from IS NULL OR d.eff_from <= $2::date) AND (d.eff_to IS NULL OR $2::date < d.eff_to)) = $4::boolean`
const contextScope = ` AND (d.class <> 'relation_context' OR d.number_norm = ANY($6::text[]))`
const exactSQL = `SELECT c.id, 1-(c.embedding <=> $1::vector) AS cosine FROM chunk c JOIN document d ON d.id=c.document_id WHERE ` + eligibility + contextScope + ` ORDER BY c.embedding <=> $1::vector, c.id LIMIT $5`
const annSQL = `WITH pool AS MATERIALIZED (SELECT id, document_id, embedding <=> $1::vector AS distance FROM chunk ORDER BY embedding <=> $1::vector LIMIT $7) SELECT p.id, 1-p.distance FROM pool p JOIN document d ON d.id=p.document_id WHERE ` + eligibility + contextScope + ` ORDER BY p.distance,p.id LIMIT $5`

func denseVector(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
func denseQuery(ctx context.Context, tx pgx.Tx, query string, args ...any) ([]Candidate, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, databaseFailure("dense query failed", err)
	}
	defer rows.Close()
	var result []Candidate
	for rows.Next() {
		var c Candidate
		if err = rows.Scan(&c.ChunkID, &c.Cosine); err != nil {
			return nil, databaseFailure("read dense candidate failed", err)
		}
		c.Rank = len(result) + 1
		result = append(result, c)
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read dense candidates failed", err)
	}
	return result, nil
}
func setLocal(ctx context.Context, tx pgx.Tx, key, value string) error {
	if _, err := tx.Exec(ctx, `SELECT set_config($1,$2,true)`, key, value); err != nil {
		return fmt.Errorf("set retrieval transaction option: %w", databaseFailure("database setting failed", err))
	}
	return nil
}
func exactSettings(ctx context.Context, tx pgx.Tx) error {
	for _, k := range []string{"enable_indexscan", "enable_bitmapscan"} {
		if err := setLocal(ctx, tx, k, "off"); err != nil {
			return err
		}
	}
	return nil
}

// indexSettings re-enables index scans after exact search so hydration reads
// only the rows its ids reach.
func indexSettings(ctx context.Context, tx pgx.Tx) error {
	for _, k := range []string{"enable_indexscan", "enable_bitmapscan"} {
		if err := setLocal(ctx, tx, k, "on"); err != nil {
			return err
		}
	}
	return nil
}
func annSettings(ctx context.Context, tx pgx.Tx, depth int, r ANNRecipe) (int, error) {
	pool := max(r.PoolMin, r.PoolMultiplier*depth)
	ef := min(pool, r.EFMax)
	for _, s := range [][2]string{{"hnsw.ef_search", strconv.Itoa(ef)}, {"hnsw.iterative_scan", r.IterativeScan}, {"hnsw.max_scan_tuples", strconv.Itoa(r.MaxScanTuples)}, {"hnsw.scan_mem_multiplier", strconv.Itoa(r.ScanMemMultiplier)}, {"plan_cache_mode", r.PlanCacheMode}} {
		if err := setLocal(ctx, tx, s[0], s[1]); err != nil {
			return 0, err
		}
	}
	return pool, nil
}
