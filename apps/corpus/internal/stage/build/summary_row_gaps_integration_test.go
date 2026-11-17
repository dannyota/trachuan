//go:build integration

package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestLoad_SummaryRowCapPersistsBothGaps(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	key := "vn:chinhphu:1"
	url := "https://example.test/" + strings.Repeat("a", 65536)
	html := []byte("detail")
	metadata, err := json.Marshal(map[string]any{"version": 1, "source_id": "1", "source_url": url, "occurrences": []any{}, "attachments": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{"version": 2, "document": map[string]any{
		"key": key, "country": "vn", "source": "chinhphu", "source_id": "1",
		"number": "SECONDARY-1", "title": "Title", "doc_type": "Decree", "issuer": "Government",
		"raw_issue_date": "2024-01-02", "source_url": url,
		"identity_evidence": []any{
			map[string]any{"path": "detail.html", "sha256": hashBytes(html)},
			map[string]any{"path": "metadata.json", "sha256": hashBytes(metadata)},
		},
	}}
	f.addArtifact(t, "fetch", key, map[string]any{
		"document.json": document, "metadata.json": metadata, "detail.html": html,
		"files.json": map[string]any{"version": 2, "files": []any{}},
	})
	f.fetch(t, "vn:vbpl:2")
	inv := f.inventory(t)
	inv.Version = 2
	inv.Membership.Entries = []membership.Entry{
		{DocumentKey: key, Disposition: "active", Class: "secondary", Source: "chinhphu"},
		{DocumentKey: "vn:vbpl:2", Disposition: "active", Class: "primary", Source: "vbpl"},
	}
	counts, err := Load(ctx, f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	if counts.Documents != 2 || counts.Summary.OutlineDropped != 1 {
		t.Fatalf("counts=%+v", counts)
	}
	var total, distinct, first, last int64
	err = f.corpus.QueryRow(ctx, `SELECT count(*),count(DISTINCT id),min(id),max(id) FROM coverage_gap`).Scan(&total, &distinct, &first, &last)
	if err != nil || total <= 2 || total != counts.Gaps || distinct != total || first != 1 || last != total {
		t.Fatalf("gap IDs total=%d distinct=%d first=%d last=%d count=%d: %v", total, distinct, first, last, counts.Gaps, err)
	}
	var outlines, rows, summaries int
	var sameDocument bool
	err = f.corpus.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM coverage_gap WHERE kind='summary_outline_capped' AND document_key=$1 AND stage='build' AND detail=''),
 (SELECT count(*) FROM coverage_gap WHERE kind='summary_row_capped' AND document_key=$1 AND stage='build' AND detail=''),
 (SELECT bool_and(g.document_id=d.id) FROM coverage_gap g JOIN document d ON d.key=g.document_key WHERE g.document_key=$1 AND g.kind IN ('summary_outline_capped','summary_row_capped')),
 (SELECT count(*) FROM document_summary WHERE document_key=$1)`, key).Scan(&outlines, &rows, &sameDocument, &summaries)
	if err != nil || outlines != 1 || rows != 1 || !sameDocument || summaries != 0 {
		t.Fatalf("outlines=%d rows=%d same document=%t summaries=%d: %v", outlines, rows, sameDocument, summaries, err)
	}
	var kept string
	if err = f.corpus.QueryRow(ctx, `SELECT document_key FROM document_summary`).Scan(&kept); err != nil || kept != "vn:vbpl:2" {
		t.Fatalf("remaining summary=%q: %v", kept, err)
	}
}
