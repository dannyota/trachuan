package evidence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type releaseDocument struct {
	ID                                                                                                   int64
	Key, Number, NumberNorm, Title, StatusClass, TextSource, ExtractSHA256, Issuer, RawIssueDate, Source string
	Baseline, ProvisionTargeted, TextRelation, Consolidated, HasProvisions                               bool
}

type releaseProvision struct {
	ID                                         int64
	ParentID                                   *int64
	NodeID, Kind, Label, Heading, Anchor, Text string
	Ordinal                                    int
}

type releaseAnchor struct {
	ID     int64
	Anchor string
}

type relationEnds struct {
	TargetDocument int64
	TargetNode     string
	Kind           string
}

type releaseData interface {
	Documents(context.Context, func(releaseDocument) error) error
	ExtractGaps(context.Context) (map[string]map[string]bool, error)
	Articles(context.Context, func(int64, []releaseProvision) error) error
	Anchors(context.Context, int64) ([]releaseAnchor, error)
	ProvisionRelations(context.Context) (map[relationEnds][]int64, error)
	PairRelations(context.Context, func(pairRelation) error) error
	TextGaps(context.Context) (map[string]bool, error)
}

type pairRelation struct{ Source, Target, Kind, Method string }

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type sqlRelease struct{ q queryer }

const documentsSQL = `SELECT d.id, d.key, d.number, d.number_norm, d.title, d.status_class, d.text_source, COALESCE(d.extract_sha256,''), d.issuer, d.raw_issue_date, d.source, d.is_consolidated,
 EXISTS (SELECT 1 FROM coverage_gap g WHERE g.stage='build' AND g.kind='baseline_served' AND g.item_key=d.key),
 EXISTS (SELECT 1 FROM relation r WHERE r.target_document_id=d.id AND r.target_provision_id IS NOT NULL),
 EXISTS (SELECT 1 FROM provision p WHERE p.document_id=d.id),
 EXISTS (SELECT 1 FROM relation r WHERE r.source_document_id=d.id AND starts_with(r.method,'text_rule/'))
 FROM document d ORDER BY d.id`

func (s sqlRelease) Documents(ctx context.Context, fn func(releaseDocument) error) error {
	rows, err := s.q.Query(ctx, documentsSQL)
	if err != nil {
		return fmt.Errorf("read documents: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d releaseDocument
		if err = rows.Scan(&d.ID, &d.Key, &d.Number, &d.NumberNorm, &d.Title, &d.StatusClass, &d.TextSource, &d.ExtractSHA256, &d.Issuer, &d.RawIssueDate, &d.Source, &d.Consolidated, &d.Baseline, &d.ProvisionTargeted, &d.HasProvisions, &d.TextRelation); err != nil {
			return fmt.Errorf("read documents: %w", err)
		}
		if err = fn(d); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("read documents: %w", err)
	}
	return nil
}

func (s sqlRelease) ExtractGaps(ctx context.Context) (map[string]map[string]bool, error) {
	rows, err := s.q.Query(ctx, `SELECT item_key, kind FROM coverage_gap WHERE stage='extract'`)
	if err != nil {
		return nil, fmt.Errorf("read extract gaps: %w", err)
	}
	defer rows.Close()
	out := map[string]map[string]bool{}
	for rows.Next() {
		var key, kind string
		if err = rows.Scan(&key, &kind); err != nil {
			return nil, fmt.Errorf("read extract gaps: %w", err)
		}
		if out[key] == nil {
			out[key] = map[string]bool{}
		}
		out[key][kind] = true
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read extract gaps: %w", err)
	}
	return out, nil
}

func (s sqlRelease) Articles(ctx context.Context, fn func(int64, []releaseProvision) error) error {
	rows, err := s.q.Query(ctx, `SELECT document_id, id, parent_id, node_id, kind, label, heading, anchor, text, ordinal FROM provision ORDER BY document_id, preorder`)
	if err != nil {
		return fmt.Errorf("read provisions: %w", err)
	}
	defer rows.Close()
	var current int64
	var group []releaseProvision
	flush := func() error {
		if len(group) == 0 {
			return nil
		}
		callErr := fn(current, group)
		group = nil
		return callErr
	}
	for rows.Next() {
		var doc int64
		var p releaseProvision
		if err = rows.Scan(&doc, &p.ID, &p.ParentID, &p.NodeID, &p.Kind, &p.Label, &p.Heading, &p.Anchor, &p.Text, &p.Ordinal); err != nil {
			return fmt.Errorf("read provisions: %w", err)
		}
		if doc != current {
			if err = flush(); err != nil {
				return err
			}
			current = doc
		}
		group = append(group, p)
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("read provisions: %w", err)
	}
	return flush()
}

func (s sqlRelease) Anchors(ctx context.Context, documentID int64) ([]releaseAnchor, error) {
	rows, err := s.q.Query(ctx, `SELECT id, anchor FROM provision WHERE document_id=$1`, documentID)
	if err != nil {
		return nil, fmt.Errorf("read anchors: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (releaseAnchor, error) {
		var a releaseAnchor
		return a, r.Scan(&a.ID, &a.Anchor)
	})
	if err != nil {
		return nil, fmt.Errorf("read anchors: %w", err)
	}
	return out, nil
}

func (s sqlRelease) ProvisionRelations(ctx context.Context) (map[relationEnds][]int64, error) {
	rows, err := s.q.Query(ctx, `SELECT source_provision_id, target_document_id, target_node_id, kind FROM relation WHERE source_provision_id IS NOT NULL AND target_document_id IS NOT NULL AND target_node_id IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("read relations: %w", err)
	}
	defer rows.Close()
	out := map[relationEnds][]int64{}
	for rows.Next() {
		var source int64
		var e relationEnds
		if err = rows.Scan(&source, &e.TargetDocument, &e.TargetNode, &e.Kind); err != nil {
			return nil, fmt.Errorf("read relations: %w", err)
		}
		out[e] = append(out[e], source)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read relations: %w", err)
	}
	return out, nil
}

func (s sqlRelease) PairRelations(ctx context.Context, fn func(pairRelation) error) error {
	rows, err := s.q.Query(ctx, `SELECT source_key, target_key, kind, method FROM relation WHERE target_key IS NOT NULL AND source_document_id IS NOT NULL ORDER BY source_key, target_key, id`)
	if err != nil {
		return fmt.Errorf("read relation pairs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p pairRelation
		if err = rows.Scan(&p.Source, &p.Target, &p.Kind, &p.Method); err != nil {
			return fmt.Errorf("read relation pairs: %w", err)
		}
		if err = fn(p); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("read relation pairs: %w", err)
	}
	return nil
}

func (s sqlRelease) TextGaps(ctx context.Context) (map[string]bool, error) {
	rows, err := s.q.Query(ctx, `SELECT DISTINCT item_key FROM coverage_gap WHERE kind = ANY($1)`, textRowGapKinds())
	if err != nil {
		return nil, fmt.Errorf("read text gaps: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read text gaps: %w", err)
	}
	out := make(map[string]bool, len(keys))
	for _, key := range keys {
		out[key] = true
	}
	return out, nil
}
