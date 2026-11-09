package build

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

func sparseText(v lexical.Vector) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, e := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(int64(e.Index), 10))
		b.WriteByte(':')
		b.WriteString(strconv.FormatFloat(float64(e.Value), 'g', -1, 32))
	}
	b.WriteString("}/1048576")
	return b.String()
}

type chunkText struct {
	id   int64
	text string
}
type textIterator struct {
	tx    pgx.Tx
	after int64
	page  []chunkText
	done  bool
}

func (it *textIterator) fill(ctx context.Context) error {
	rows, err := it.tx.Query(ctx, `SELECT id, content || ' ' || context_prefix FROM chunk WHERE id > $1 ORDER BY id LIMIT 2000`, it.after)
	if err != nil {
		return fmt.Errorf("query lexical chunks: %w", err)
	}
	defer rows.Close()
	it.page = nil
	for rows.Next() {
		var row chunkText
		if err = rows.Scan(&row.id, &row.text); err != nil {
			return fmt.Errorf("scan lexical chunk: %w", err)
		}
		it.page = append(it.page, row)
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("read lexical chunks: %w", err)
	}
	if len(it.page) == 0 {
		it.done = true
	}
	return nil
}
func (it *textIterator) Next(ctx context.Context) (string, bool, error) {
	if len(it.page) == 0 && !it.done {
		if err := it.fill(ctx); err != nil {
			return "", false, err
		}
	}
	if len(it.page) == 0 {
		return "", false, nil
	}
	row := it.page[0]
	it.page = it.page[1:]
	it.after = row.id
	return row.text, true, nil
}

func loadLexical(ctx context.Context, tx pgx.Tx, recipe lexical.Recipe) (lexical.StatsSummary, error) {
	stats, err := lexical.Train(ctx, &textIterator{tx: tx}, recipe)
	if err != nil {
		return lexical.StatsSummary{}, fmt.Errorf("train lexical index: %w", err)
	}
	if len(stats.DF) > 2000000 {
		return lexical.StatsSummary{}, fmt.Errorf("lexical term limit exceeded")
	}
	terms := make([]string, 0, len(stats.DF))
	for term := range stats.DF {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	for start := 0; start < len(terms); start += 2000 {
		batch := &pgx.Batch{}
		end := min(start+2000, len(terms))
		for _, term := range terms[start:end] {
			batch.Queue(`INSERT INTO lexical_df(token,dimension,df) VALUES($1,$2,$3)`, term, lexical.Dimension(term, recipe), stats.DF[term])
		}
		if err = tx.SendBatch(ctx, batch).Close(); err != nil {
			return lexical.StatsSummary{}, fmt.Errorf("insert lexical frequencies: %w", err)
		}
	}
	it := &textIterator{tx: tx}
	nnz := make([]int, 0, int(stats.N))
	for !it.done {
		if err = it.fill(ctx); err != nil {
			return lexical.StatsSummary{}, err
		}
		batch := &pgx.Batch{}
		for _, row := range it.page {
			v, encodeErr := lexical.Document(row.text, stats, recipe)
			if encodeErr != nil {
				return lexical.StatsSummary{}, fmt.Errorf("encode lexical document: %w", encodeErr)
			}
			batch.Queue(`UPDATE chunk SET sparse=$2::sparsevec WHERE id=$1`, row.id, sparseText(v))
			nnz = append(nnz, len(v))
			it.after = row.id
		}
		if len(it.page) > 0 {
			if err = tx.SendBatch(ctx, batch).Close(); err != nil {
				return lexical.StatsSummary{}, fmt.Errorf("write lexical vectors: %w", err)
			}
		}
		it.page = nil
	}
	return stats.Summary(recipe, nnz), nil
}
