package retrieval

import (
	"context"
	"errors"
	"reflect"
	"time"

	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

func (e *Engine) smokePostings(ctx context.Context, tx pgx.Tx, index *sparsePostings) error {
	rows, err := tx.Query(ctx, `SELECT title FROM document ORDER BY id LIMIT 20`)
	if err != nil {
		return databaseFailure("read postings probes failed", err)
	}
	var probes []string
	for rows.Next() {
		var title string
		if err = rows.Scan(&title); err != nil {
			rows.Close()
			return databaseFailure("read postings probe failed", err)
		}
		probes = append(probes, title)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return databaseFailure("read postings probes failed", err)
	}
	today, err := time.Parse(time.DateOnly, e.searchDate(Request{}))
	if err != nil {
		return errors.New("invalid postings probe date")
	}
	depth := e.release.RetrievalRecipe.LexicalDepth
	for _, title := range probes {
		query, queryErr := lexical.Query(title, e.release.LexicalRecipe)
		validation, validationErr := ValidateQuery(title)
		if queryErr != nil || validationErr != nil {
			return errors.New("invalid postings probe")
		}
		refs := append([]string{}, validation.References...)
		sv := sparseVector(query, e.release.LexicalRecipe.Dimensions)
		for _, historical := range []bool{false, true} {
			date := today
			if historical {
				date = today.AddDate(-1, 0, 0)
			}
			got, scoreErr := index.lexical(ctx, tx, query, sv, date, historical, false, refs, depth)
			if scoreErr != nil {
				return scoreErr
			}
			for i := range got {
				want, referenceErr := lexicalQuery(ctx, tx, sv, date.Format(time.DateOnly), historical, i == 0, depth, refs)
				if referenceErr != nil {
					return referenceErr
				}
				if !reflect.DeepEqual(got[i], want) {
					return errors.New("postings probe differs from SQL")
				}
			}
		}
	}
	return nil
}
