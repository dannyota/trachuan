//go:build integration

package build

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestLoad_SummaryCandidatesWithoutApproval(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	first := addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CCHL", "2024-01-01", "2027-01-01", "Nghị định này có hiệu lực từ ngày 1/7/2024.")
	f.fetch(t, "vn:vbpl:2")
	f.addArtifact(t, "relate", "vn:vbpl:1", map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "cites", "target_vbpl_id": "2"}, {"ordinal": 1, "kind": "replaces", "target_vbpl_id": "999", "target_number": "outside/2024"}}}, f.relateInput(t, "vn:vbpl:1", first))
	input := taxonomyInputFixture()
	input.seeds = []taxonomySeed{{"1", "1", "1.1.c1"}}
	_, err := Load(ctx, f.corpus, f.inventory(t), f.artifacts, f.recipes, map[string]source.StatusCode{"CCHL": {Class: "not_yet"}}, nil, f.asOf(), LoadOptions{Taxonomy: input})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	var equal bool
	var raw []byte
	err = f.corpus.QueryRow(ctx, `SELECT (SELECT count(*) FROM document_summary),s.payload,(s.status_class,s.status_basis,s.eff_from,s.eff_to,s.is_current,s.validity_conflict,s.class) IS NOT DISTINCT FROM (d.status_class,d.status_basis,d.eff_from,d.eff_to,d.is_current,d.validity_conflict,d.class) FROM document_summary s JOIN document d ON d.key=s.document_key WHERE d.key='vn:vbpl:1' AND s.indexed`).Scan(&count, &raw, &equal)
	if err != nil || count != 2 || !equal {
		t.Fatalf("rows=%d equal=%v: %v", count, equal, err)
	}
	var p summaryPayload
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Outline) != 1 || p.Outline[0].Heading != "Hiệu lực thi hành" || len(p.Nodes) != 1 || p.Nodes[0].Name != "Chapter" {
		t.Fatalf("payload: %s", raw)
	}
	if p.RelationsTotal != 2 || len(p.Relations) != 2 || p.Relations[0].Key == nil || *p.Relations[0].Key != "vn:vbpl:2" || p.Relations[0].Number != "01/2024/TT" || p.Relations[1].Key != nil || p.Relations[1].Number != "outside/2024" {
		t.Fatalf("summary pointers: %s", raw)
	}
	if _, err = f.corpus.Exec(ctx, `UPDATE document_summary SET payload=jsonb_set(payload,'{outline,0,heading}','"phải làm"') WHERE document_key='vn:vbpl:1'`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.corpus.Exec(ctx, `SET default_transaction_read_only=on`); err != nil {
		t.Fatal(err)
	}
	scan, err := HeadingScan(ctx, f.corpus, "vn", f.run.Rel, nil)
	if err != nil || !strings.Contains(string(scan), `"heading":"phải làm"`) {
		t.Fatalf("unapproved scan: %s %v", scan, err)
	}
}

func TestLoad_SummaryExclusionOutsideRelease(t *testing.T) {
	for _, key := range []string{"vn:vbpl:1", "vn:vbpl:999"} {
		t.Run(key, func(t *testing.T) {
			f := fixture(t)
			f.fetch(t, "vn:vbpl:1")
			_, err := Load(context.Background(), f.corpus, f.inventory(t), f.artifacts, f.recipes, nil, nil, f.asOf(), LoadOptions{HeadingExclusions: []source.HeadingExclusion{{Key: key, Anchor: "dieu-1", Reason: "operative", Reviewed: "2026-11-16"}}})
			if err == nil || !strings.Contains(err.Error(), "absent from release") {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

func TestLoad_SummaryOutlineCappedGap(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	art := f.fetch(t, "vn:vbpl:1")
	nodes := []chunk.Node{}
	for i := 1; i <= 2201; i++ {
		nodes = append(nodes, chunk.Node{NodeID: fmt.Sprint(i), Kind: "dieu", Label: "Điều " + fmt.Sprint(i), Path: "dieu-" + fmt.Sprint(i), Ordinal: i, Heading: "Title", Text: "Body"})
	}
	f.addArtifact(t, "extract", "vn:vbpl:1", map[string]any{"provisions.json": nodes, "report.json": []any{}}, art.SHA256)
	if _, err := f.load(t); err != nil {
		t.Fatal(err)
	}
	var indexed bool
	var outlines, gaps int
	err := f.corpus.QueryRow(ctx, `SELECT indexed,jsonb_array_length(payload->'outline'),(SELECT count(*) FROM coverage_gap WHERE stage='build' AND kind='summary_outline_capped' AND document_key='vn:vbpl:1') FROM document_summary`).Scan(&indexed, &outlines, &gaps)
	if err != nil || indexed || outlines != 0 || gaps != 1 {
		t.Fatalf("indexed=%v outline=%d gaps=%d: %v", indexed, outlines, gaps, err)
	}
}

func TestLoad_UnknownSummaryGapFailsBuild(t *testing.T) {
	for _, withDocument := range []bool{true, false} {
		t.Run(fmt.Sprint(withDocument), func(t *testing.T) {
			f := fixture(t)
			if withDocument {
				f.fetch(t, "vn:vbpl:1")
			}
			inv := f.inventory(t)
			inv.Gaps = append(inv.Gaps, InventoryGap{Stage: "extract", Kind: "future_structure_gap"})
			_, err := Load(context.Background(), f.corpus, inv, f.artifacts, f.recipes, nil, nil, f.asOf())
			if err == nil || !strings.Contains(err.Error(), "future_structure_gap") {
				t.Fatalf("unknown gap accepted: %v", err)
			}
			var count int
			if err = f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM document_summary`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed build left %d summaries: %v", count, err)
			}
		})
	}
}
