package retrieval

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
)

type sparseTriple struct {
	dimension int32
	position  uint32
	weight    float32
}
type sparseSize struct{ chunks, postings, documents, textBytes int64 }

func (s sparseSize) estimate(dimensions int) (steady, peak int64, ok bool) {
	if s.chunks < 0 || s.chunks > math.MaxUint32 || s.postings < 0 || s.postings > math.MaxUint32 || s.documents < 0 || s.documents > math.MaxUint32 || s.textBytes < 0 || s.textBytes > 600_000_000 {
		return 0, 0, false
	}
	directory := min(s.postings, int64(dimensions)) * 12
	steady = s.postings*8 + directory + s.chunks*16 + s.documents*192 + s.textBytes
	peak = steady*2 + s.postings*12 + s.chunks*80 + s.documents*64
	return steady, peak, peak <= 600_000_000
}

func (e *Engine) loadPostings(ctx context.Context) (*sparsePostings, PostingsOutcome) {
	var outcome PostingsOutcome
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		outcome.Reason = "begin load failed"
		return nil, outcome
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var size sparseSize
	err = tx.QueryRow(ctx, `SELECT count(*), COALESCE(sum(length(sparse::text)-length(replace(sparse::text,':',''))),0)::bigint FROM chunk`).Scan(&size.chunks, &size.postings)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT count(*), COALESCE(sum(octet_length(class)+octet_length(number_norm)+octet_length(status_class)),0)::bigint FROM document`).Scan(&size.documents, &size.textBytes)
	}
	if err != nil {
		outcome.Reason = "read load counts failed"
		return nil, outcome
	}
	var allowed bool
	outcome.Bytes, outcome.EstimatedPeakBytes, allowed = size.estimate(e.release.LexicalRecipe.Dimensions)
	if !allowed {
		outcome.Reason = "estimated peak exceeds 600 MB or invalid counts"
		return nil, outcome
	}
	index, err := e.buildPostings(ctx, tx, size)
	if err != nil {
		outcome.Reason = "invalid postings snapshot or decoding failed"
		return nil, outcome
	}
	if err = e.smokePostings(ctx, tx, index); err != nil {
		outcome.Reason = "smoke check failed"
		return nil, outcome
	}
	if err = tx.Commit(ctx); err != nil {
		outcome.Reason = "commit load failed"
		return nil, outcome
	}
	return index, outcome
}

func (e *Engine) buildPostings(ctx context.Context, tx pgx.Tx, size sparseSize) (*sparsePostings, error) {
	index := &sparsePostings{chunks: make([]sparseChunk, 0, int(size.chunks)), documents: make([]sparseDocument, 0, int(size.documents))}
	documents := make(map[int64]uint32, int(size.documents))
	rows, err := tx.Query(ctx, `SELECT id,is_current,validity_conflict,eff_from,eff_to,class,number_norm,status_class FROM document ORDER BY id`)
	if err != nil {
		return nil, databaseFailure("read postings documents failed", err)
	}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		var id int64
		var doc sparseDocument
		if err = rows.Scan(&id, &doc.current, &doc.conflict, &doc.from, &doc.to, &doc.class, &doc.number, &doc.status); err != nil {
			rows.Close()
			return nil, databaseFailure("decode postings document failed", err)
		}
		if int64(len(index.documents)) >= size.documents {
			rows.Close()
			return nil, errors.New("postings document count changed")
		}
		documents[id] = uint32(len(index.documents))
		index.documents = append(index.documents, doc)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, databaseFailure("read postings documents failed", err)
	}
	triples := make([]sparseTriple, 0, int(size.postings))
	rows, err = tx.Query(ctx, `SELECT id,document_id,sparse::text FROM chunk ORDER BY id`)
	if err != nil {
		return nil, databaseFailure("read postings chunks failed", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		var id, document int64
		var text string
		if err = rows.Scan(&id, &document, &text); err != nil {
			return nil, databaseFailure("read postings chunk failed", err)
		}
		doc, found := documents[document]
		if !found || int64(len(index.chunks)) >= size.chunks {
			return nil, errors.New("invalid postings chunk owner or count")
		}
		vector, bound, decodeErr := decodeSparse(text, e.release.LexicalRecipe.Dimensions)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if int64(len(triples)+len(vector)) > size.postings {
			return nil, errors.New("postings entry count changed")
		}
		index.delta = max(index.delta, bound)
		pos := uint32(len(index.chunks))
		index.chunks = append(index.chunks, sparseChunk{id: id, document: doc})
		for i, entry := range vector {
			if i%4096 == 0 {
				if err = ctx.Err(); err != nil {
					return nil, err
				}
			}
			triples = append(triples, sparseTriple{dimension: entry.Index, position: pos, weight: entry.Value})
		}
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read postings chunks failed", err)
	}
	rows.Close()
	if int64(len(triples)) != size.postings || int64(len(index.chunks)) != size.chunks || int64(len(index.documents)) != size.documents {
		return nil, errors.New("postings counts differ")
	}
	if err = sortSparseTriples(ctx, triples); err != nil {
		return nil, err
	}
	if err = index.buildDirectory(ctx, triples); err != nil {
		return nil, err
	}
	return index, nil
}
