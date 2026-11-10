package membership

import (
	"context"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

// EvidenceRecord links a support tuple to its producing item and support state.
type EvidenceRecord struct {
	DocumentKey string
	Stage       string
	ItemKey     string
	Support     SupportTuple
	State       string
}

// UpsertDocument writes a run document through a transaction-bound Queries.
func UpsertDocument(ctx context.Context, q *store.Queries, runID int64, entry Entry) error {
	country, _, ok := strings.Cut(entry.DocumentKey, ":")
	if !ok {
		return fmt.Errorf("invalid document key %q", entry.DocumentKey)
	}
	err := q.UpsertRunDocument(ctx, store.UpsertRunDocumentParams{RunID: runID, DocumentKey: entry.DocumentKey, Country: country, Source: entry.Source, SourceID: entry.SourceID, Class: entry.Class, Disposition: entry.Disposition, RawNumber: entry.RawNumber, NormalizedNumber: entry.NormalizedNumber, ReplacementKey: entry.ReplacementKey})
	if err != nil {
		return fmt.Errorf("upsert run document: %w", err)
	}
	return nil
}

// RecordEvidence stores one independently produced support, deduplicating repeats.
func RecordEvidence(ctx context.Context, q *store.Queries, runID int64, record EvidenceRecord) error {
	e := record.Support.Evidence
	err := q.InsertMembershipEvidence(ctx, store.InsertMembershipEvidenceParams{RunID: runID, DocumentKey: record.DocumentKey, Stage: record.Stage, ItemKey: record.ItemKey, RuleKind: record.Support.RuleKind, RuleValue: record.Support.RuleValue, QueryIdentity: e.QueryIdentity, Page: e.Page, ResponseSha256: e.ResponseSHA256, ArtifactHash: e.ArtifactHash, EvidencePath: e.Path, SupportState: record.State})
	if err != nil {
		return fmt.Errorf("record membership evidence: %w", err)
	}
	return nil
}

// ReadDocuments reads current run documents and all their audit support tuples.
func ReadDocuments(ctx context.Context, q *store.Queries, runID int64) ([]Entry, error) {
	rows, err := q.ListRunDocuments(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("read run documents: %w", err)
	}
	evidence, err := q.ListMembershipEvidence(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("read membership evidence: %w", err)
	}
	supports := make(map[string][]SupportTuple)
	for _, row := range evidence {
		supports[row.DocumentKey] = append(supports[row.DocumentKey], SupportTuple{RuleKind: row.RuleKind, RuleValue: row.RuleValue, Evidence: EvidenceRef{QueryIdentity: row.QueryIdentity, Page: row.Page, ResponseSHA256: row.ResponseSha256, ArtifactHash: row.ArtifactHash, Path: row.EvidencePath}})
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, Entry{DocumentKey: row.DocumentKey, Source: row.Source, SourceID: row.SourceID, Class: row.Class, Disposition: row.Disposition, RawNumber: row.RawNumber, NormalizedNumber: row.NormalizedNumber, ReplacementKey: row.ReplacementKey, Supports: MergeSupports(nil, supports[row.DocumentKey])})
	}
	return entries, nil
}

// ReadEvidence preserves the producing item and support state for rebuilding membership.
func ReadEvidence(ctx context.Context, q *store.Queries, runID int64) ([]EvidenceRecord, error) {
	rows, err := q.ListMembershipEvidence(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("read membership evidence: %w", err)
	}
	records := make([]EvidenceRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, EvidenceRecord{DocumentKey: row.DocumentKey, Stage: row.Stage, ItemKey: row.ItemKey, State: row.SupportState, Support: SupportTuple{RuleKind: row.RuleKind, RuleValue: row.RuleValue, Evidence: EvidenceRef{QueryIdentity: row.QueryIdentity, Page: row.Page, ResponseSHA256: row.ResponseSha256, ArtifactHash: row.ArtifactHash, Path: row.EvidencePath}}})
	}
	return records, nil
}
