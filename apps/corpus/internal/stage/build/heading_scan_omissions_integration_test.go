//go:build integration

package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestHeadingScan_RuleThreeStoredProvisionsWithoutSummary(t *testing.T) {
	f := fixture(t)
	for _, key := range []string{"vn:vbpl:1", "vn:vbpl:2", "vn:vbpl:3"} {
		f.fetch(t, key)
	}
	if _, err := f.load(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := f.corpus.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `DELETE FROM document_summary`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM coverage_gap`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE document SET text_source='source_tree',number=CASE WHEN key='vn:vbpl:2' THEN repeat('ố',201) ELSE number END,source_url=CASE WHEN key='vn:vbpl:3' THEN 'https://example.test/'||repeat('a',65536) ELSE source_url END`); err != nil {
		t.Fatal(err)
	}
	docs := map[string]*loadedDocument{}
	for _, key := range []string{"vn:vbpl:1", "vn:vbpl:2", "vn:vbpl:3"} {
		var id int64
		if err = tx.QueryRow(ctx, `SELECT id FROM document WHERE key=$1`, key).Scan(&id); err != nil {
			t.Fatal(err)
		}
		docs[key] = &loadedDocument{id: id}
	}
	for i, tc := range []struct {
		key, kind, anchor string
		length            int
	}{
		{"vn:vbpl:1", "dieu", "at-cap", 300},
		{"vn:vbpl:1", "dieu", "over-cap", 301},
		{"vn:vbpl:1", "dieu", "excluded", 500},
		{"vn:vbpl:2", "chuong", "field-capped", 302},
		{"vn:vbpl:3", "dieu", "row-capped", 303},
	} {
		_, err = tx.Exec(ctx, `INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES($1::bigint,$2,$3,$4,'Label',$5,$1::integer,$1::integer,('node_'||$1::text)::ltree,'Body',$3)`, i+1, docs[tc.key].id, tc.anchor, tc.kind, strings.Repeat("ố", tc.length))
		if err != nil {
			t.Fatal(err)
		}
	}
	exclusions := []source.HeadingExclusion{{Key: "vn:vbpl:1", Anchor: "excluded", Reason: "operative", Reviewed: "2026-11-17"}}
	l := loader{tx: tx, docs: docs}
	if err = l.loadSummaries(ctx, exclusions); err != nil {
		t.Fatal(err)
	}
	if err = l.loadGaps(ctx); err != nil {
		t.Fatal(err)
	}
	var summaries, fields, rows int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM document_summary),(SELECT count(*) FROM coverage_gap WHERE kind='summary_field_capped'),(SELECT count(*) FROM coverage_gap WHERE kind='summary_row_capped')`).Scan(&summaries, &fields, &rows); err != nil || summaries != 1 || fields != 1 || rows != 1 {
		t.Fatalf("summaries=%d field gaps=%d row gaps=%d: %v", summaries, fields, rows, err)
	}
	b, err := HeadingScan(ctx, tx, "vn", f.run.Rel, exclusions)
	if err != nil {
		t.Fatal(err)
	}
	var got headingScanOutput
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Anchor != "at-cap" || got.Omitted.Count != 3 || len(got.Omitted.Longest) != 3 {
		t.Fatalf("scan=%s", b)
	}
	for i, anchor := range []string{"row-capped", "field-capped", "over-cap"} {
		if got.Omitted.Longest[i].Anchor != anchor || got.Omitted.Longest[i].Length != 303-i {
			t.Errorf("omission %d=%+v", i, got.Omitted.Longest[i])
		}
	}
}
