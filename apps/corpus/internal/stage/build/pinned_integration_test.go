//go:build integration

package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
)

func TestLoad_PinnedColumnsAndCoverageGap(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	key := "vn:vbpl:pin"
	art := f.addArtifact(t, "fetch", key, map[string]any{"detail.json": []byte(`{"data":{"id":"pin","docNum":"71/2099/QH15","effFrom":"2099-01-01"}}`), "files.json": []any{}})
	f.addArtifact(t, "fetch", "vn:vbpl:ordinary", map[string]any{"detail.json": []byte(`{"data":{"id":"ordinary","docNum":"72/2099/QH15","effFrom":"2099-01-01"}}`), "files.json": []any{}})
	pin := source.Pin{Key: key, Number: "71/2099/QH15", Since: "2099-01-02", Rel: "20990101_1", Expand: strings.Repeat("a", 64), Fetch: art.SHA256}
	inv := f.inventory(t)
	inv.Version = 2
	inv.Membership.Entries = []membership.Entry{
		{DocumentKey: key, Source: "vbpl", Class: "primary", Disposition: "active", Supports: []membership.SupportTuple{{RuleKind: "pinned", RuleValue: key}}},
		{DocumentKey: "vn:vbpl:ordinary", Source: "vbpl", Class: "primary", Disposition: "active"},
	}
	if _, err := Load(ctx, f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf(), LoadOptions{Pins: []source.Pin{pin}}); err != nil {
		t.Fatal(err)
	}
	var since *time.Time
	var rel *string
	if err := f.corpus.QueryRow(ctx, `SELECT source_withdrawn_since,source_withdrawn_rel FROM document WHERE key=$1`, key).Scan(&since, &rel); err != nil {
		t.Fatal(err)
	}
	if since == nil || since.Format("2006-01-02") != pin.Since || rel == nil || *rel != pin.Rel {
		t.Fatalf("since=%v rel=%v", since, rel)
	}
	var detail string
	if err := f.corpus.QueryRow(ctx, `SELECT detail FROM coverage_gap WHERE document_key=$1 AND kind='source_withdrawn_retained'`, key).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	var retained source.Pin
	if err := json.Unmarshal([]byte(detail), &retained); err != nil {
		t.Fatal(err)
	}
	if retained != pin {
		t.Fatalf("gap pin=%+v", retained)
	}
	if err := f.corpus.QueryRow(ctx, `SELECT source_withdrawn_since,source_withdrawn_rel FROM document WHERE key='vn:vbpl:ordinary'`).Scan(&since, &rel); err != nil {
		t.Fatal(err)
	}
	if since != nil || rel != nil {
		t.Fatalf("ordinary since=%v rel=%v", since, rel)
	}
}
