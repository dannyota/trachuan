package build

import (
	"context"
	"fmt"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"github.com/jackc/pgx/v5"
)

type headingOmission struct {
	Key    string `json:"key"`
	Anchor string `json:"anchor"`
	Length int    `json:"length"`
	reason string
	shape  string
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
	rows, err = db.Query(ctx, `SELECT d.key,d.text_source,p.kind,p.label,p.anchor,p.heading FROM document d JOIN provision p ON p.document_id=d.id ORDER BY d.key COLLATE "C",p.preorder`)
	if err != nil {
		return nil, fmt.Errorf("query omitted headings: %w", err)
	}
	defer rows.Close()
	type provision struct {
		key, textSource string
		node            summaryOutline
	}
	var provisions []provision
	outlines := map[string][]summaryOutline{}
	for rows.Next() {
		var p provision
		if err = rows.Scan(&p.key, &p.textSource, &p.node.Kind, &p.node.Label, &p.node.Anchor, &p.node.Heading); err != nil {
			return nil, fmt.Errorf("scan omitted heading: %w", err)
		}
		provisions = append(provisions, p)
		if p.node.Kind == "chuong" {
			outlines[p.key] = append(outlines[p.key], p.node)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read omitted headings: %w", err)
	}
	broken := map[string]bool{}
	for key, outline := range outlines {
		broken[key] = !chapterSequenceOK(outline)
	}
	omissions := []headingOmission{}
	for _, p := range provisions {
		n := p.node
		if p.textSource != "source_tree" || structural[p.key] || (n.Kind != "chuong" && n.Kind != "dieu") || excluded[[2]string{p.key, n.Anchor}] || n.Heading == "" {
			continue
		}
		o := headingOmission{Key: p.key, Anchor: n.Anchor, Length: utf8.RuneCountInString(n.Heading)}
		switch {
		case broken[p.key]:
			o.reason = "sequence"
		case o.Length > 300:
			o.reason = "length"
		case headingShape(n.Heading) != "":
			o.reason, o.shape = "shape", headingShape(n.Heading)
		default:
			continue
		}
		omissions = append(omissions, o)
	}
	return omissions, nil
}
