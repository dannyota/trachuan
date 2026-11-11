package eval

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"github.com/jackc/pgx/v5"
)

type documentCountQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readDocumentCounts(ctx context.Context, db documentCountQuerier, report *evaluation.Report) error {
	report.DocumentsByClass = map[string]int{}
	report.DocumentsBySource = map[string]int{}
	report.DocumentsByTextSource = map[string]int{}
	report.DocumentsByTextSourceAndClass = map[string]map[string]int{}
	report.DocumentsTextIncomplete = 0
	rows, err := db.Query(ctx, `SELECT class, source, text_source, text_complete, count(*) FROM document GROUP BY class, source, text_source, text_complete`)
	if err != nil {
		return fmt.Errorf("count documents: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var class, source, textSource string
		var complete bool
		var count int
		if err = rows.Scan(&class, &source, &textSource, &complete, &count); err != nil {
			return fmt.Errorf("scan document counts: %w", err)
		}
		report.DocumentsByClass[class] += count
		report.DocumentsBySource[source] += count
		report.DocumentsByTextSource[textSource] += count
		if report.DocumentsByTextSourceAndClass[textSource] == nil {
			report.DocumentsByTextSourceAndClass[textSource] = map[string]int{}
		}
		report.DocumentsByTextSourceAndClass[textSource][class] += count
		if !complete {
			report.DocumentsTextIncomplete += count
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("read document counts: %w", err)
	}
	return nil
}
