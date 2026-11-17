package build

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"github.com/jackc/pgx/v5"
)

type headingOmission struct {
	Key    string `json:"key"`
	Anchor string `json:"anchor"`
	Length int    `json:"length"`
}

func scanHeadingOmissions(ctx context.Context, db summaryReader, exclusions []source.HeadingExclusion) ([]headingOmission, error) {
	rows, err := db.Query(ctx, `SELECT COALESCE(document_key,''),kind FROM coverage_gap`)
	if err != nil {
		return nil, fmt.Errorf("query heading scan gaps: %w", err)
	}
	gaps, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (InventoryGap, error) {
		var gap InventoryGap
		scanErr := row.Scan(&gap.ItemKey, &gap.Kind)
		return gap, scanErr
	})
	if err != nil {
		return nil, fmt.Errorf("read heading scan gaps: %w", err)
	}
	structural := map[string]bool{}
	for _, gap := range gaps {
		blocked, e := structuralSummaryGap(gap.Kind)
		if e != nil {
			return nil, fmt.Errorf("heading scan gap: %w", e)
		}
		if blocked {
			structural[gap.ItemKey] = true
		}
	}
	excluded := map[[2]string]bool{}
	for _, entry := range exclusions {
		excluded[[2]string{entry.Key, entry.Anchor}] = true
	}
	rows, err = db.Query(ctx, `SELECT d.key,d.text_source,p.kind,p.anchor,char_length(p.heading) FROM document d JOIN provision p ON p.document_id=d.id`)
	if err != nil {
		return nil, fmt.Errorf("query omitted headings: %w", err)
	}
	defer rows.Close()
	omissions := []headingOmission{}
	for rows.Next() {
		var omission headingOmission
		var textSource, kind string
		if err = rows.Scan(&omission.Key, &textSource, &kind, &omission.Anchor, &omission.Length); err != nil {
			return nil, fmt.Errorf("scan omitted heading: %w", err)
		}
		if textSource != "source_tree" || structural[omission.Key] || (kind != "chuong" && kind != "dieu") || excluded[[2]string{omission.Key, omission.Anchor}] || omission.Length <= 300 {
			continue
		}
		omissions = append(omissions, omission)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read omitted headings: %w", err)
	}
	return omissions, nil
}
