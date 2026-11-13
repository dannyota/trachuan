package build

import (
	"context"
	"fmt"
	"time"
)

type consolidatedDocument struct {
	key, class, basis string
	issue             *time.Time
	consolidated      bool
}

func consolidatedStatus(issue *time.Time, current bool, targets int, competitors []*time.Time) (string, string) {
	newer := false
	for _, other := range competitors {
		if issue == nil || other == nil || issue.Equal(*other) {
			return "unknown", "consolidated_status_unordered"
		}
		if other.After(*issue) {
			newer = true
		}
	}
	if targets == 0 {
		return "unknown", "consolidated_status_unresolved"
	}
	if current && !newer {
		return "in_force", ""
	}
	return "expired", ""
}

func (l *loader) deriveConsolidated(ctx context.Context) error {
	if err := l.batch.flush(ctx); err != nil {
		return err
	}
	rows, err := l.tx.Query(ctx, `SELECT id,key,status_class,status_basis,issue_date,is_consolidated FROM document`)
	if err != nil {
		return fmt.Errorf("read consolidated documents: %w", err)
	}
	docs := map[int64]consolidatedDocument{}
	for rows.Next() {
		var id int64
		var d consolidatedDocument
		if err = rows.Scan(&id, &d.key, &d.class, &d.basis, &d.issue, &d.consolidated); err != nil {
			rows.Close()
			return fmt.Errorf("scan consolidated document: %w", err)
		}
		docs[id] = d
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("read consolidated documents: %w", err)
	}
	rows, err = l.tx.Query(ctx, `SELECT source_document_id,target_document_id FROM relation WHERE kind='consolidates' AND source_document_id IS NOT NULL AND target_document_id IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("read consolidation relations: %w", err)
	}
	targets := map[int64]map[int64]bool{}
	competitors := map[int64]map[int64]bool{}
	for rows.Next() {
		var source, target int64
		if err = rows.Scan(&source, &target); err != nil {
			rows.Close()
			return fmt.Errorf("scan consolidation relation: %w", err)
		}
		if !docs[source].consolidated {
			continue
		}
		if targets[source] == nil {
			targets[source] = map[int64]bool{}
		}
		targets[source][target] = true
		if competitors[target] == nil {
			competitors[target] = map[int64]bool{}
		}
		competitors[target][source] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("read consolidation relations: %w", err)
	}
	for id, doc := range docs {
		if doc.basis != "derived_from_consolidated" {
			continue
		}
		current := false
		others := map[int64]bool{}
		for target := range targets[id] {
			d := docs[target]
			current = current || (d.basis == "source_status" && (d.class == "in_force" || d.class == "partial"))
			for other := range competitors[target] {
				if other != id {
					others[other] = true
				}
			}
		}
		dates := make([]*time.Time, 0, len(others))
		for other := range others {
			dates = append(dates, docs[other].issue)
		}
		class, gap := consolidatedStatus(doc.issue, current, len(targets[id]), dates)
		if err = l.batch.queue(ctx, "update consolidated status", `UPDATE document SET status_class=$1,is_current=$2 WHERE id=$3`, class, class == "in_force", id); err != nil {
			return err
		}
		if gap != "" {
			l.addGap("build", doc.key, gap, "")
		}
	}
	return nil
}
