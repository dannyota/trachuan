package evaluate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func mutateBodyIndex(ctx context.Context, tx pgx.Tx, ids []int64) error {
	before, err := bodyChunkCounts(ctx, tx, ids)
	if err != nil {
		return err
	}
	var maxID int64
	var dimensions int
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(id),0) FROM body_chunk`).Scan(&maxID); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT atttypmod FROM pg_attribute WHERE attrelid='body_chunk'::regclass AND attname='embedding' AND NOT attisdropped`).Scan(&dimensions); err != nil {
		return err
	}
	if dimensions <= 0 {
		return fmt.Errorf("invalid mutation vector dimensions")
	}
	vector := make([]float32, dimensions)
	vector[0] = 1
	encoded, err := json.Marshal(vector)
	if err != nil {
		return err
	}
	queries := []string{
		`UPDATE body_chunk b SET content='invariantreplacement',token_count=token_count+7,embedding=(SELECT array_agg(-entry ORDER BY ord)::vector FROM unnest(b.embedding::real[]) WITH ORDINALITY AS t(entry,ord)) WHERE version_id=ANY($1::bigint[])`,
		`DELETE FROM body_posting WHERE version_id=ANY($1::bigint[])`,
		`DELETE FROM body_chunk b USING (SELECT version_id,min(id) AS id FROM body_chunk WHERE version_id=ANY($1::bigint[]) GROUP BY version_id) removed WHERE b.id=removed.id`,
	}
	for _, query := range queries {
		if _, err = tx.Exec(ctx, query, ids); err != nil {
			return err
		}
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("invariantreplacement")))
	_, err = tx.Exec(ctx, `WITH seeds AS (
 SELECT v.id AS version_id,c.id AS control_id,coalesce((SELECT max(ordinal) FROM body_chunk WHERE control_id=c.id),-1) AS last_ordinal
 FROM framework_version v JOIN LATERAL (SELECT id FROM control WHERE version_id=v.id ORDER BY ordinal,id LIMIT 1) c ON true
 WHERE v.id=ANY($1::bigint[])
 ), additions AS (
 SELECT s.*,extra.number,row_number() OVER(ORDER BY s.version_id,extra.number) AS sequence
 FROM seeds s CROSS JOIN generate_series(1,2) extra(number)
 ) INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
 SELECT $2::bigint+sequence,version_id,control_id,last_ordinal+number,'invariantreplacement',$3,8,$4::vector FROM additions`, ids, maxID, hash, string(encoded))
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO body_posting(version_id,chunk_id,token,tf) SELECT version_id,id,'invariantreplacement',token_count FROM body_chunk WHERE version_id=ANY($1::bigint[])`, ids); err != nil {
		return err
	}
	after, err := bodyChunkCounts(ctx, tx, ids)
	if err != nil {
		return err
	}
	for id, count := range before {
		if after[id] == count {
			return fmt.Errorf("body index cardinality unchanged")
		}
	}
	return nil
}
func bodyChunkCounts(ctx context.Context, tx pgx.Tx, ids []int64) (map[int64]int, error) {
	rows, err := tx.Query(ctx, `SELECT v.id,count(b.id) FROM framework_version v LEFT JOIN body_chunk b ON b.version_id=v.id WHERE v.id=ANY($1::bigint[]) GROUP BY v.id ORDER BY v.id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[int64]int{}
	for rows.Next() {
		var id int64
		var count int
		if err = rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	return counts, rows.Err()
}
