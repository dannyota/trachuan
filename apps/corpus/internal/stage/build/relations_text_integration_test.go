//go:build integration

package build

import (
	"context"
	"fmt"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
)

func TestLoad_TextRelationAnchors(t *testing.T) {
	f := fixture(t)
	sourceFetch := f.fetch(t, "vn:vbpl:1")
	f.addArtifact(t, "extract", "vn:vbpl:1", map[string]any{"provisions.json": []chunk.Node{{NodeID: "source", Kind: "dieu", Ordinal: 1, Path: "dieu-1", Text: "Ngưng hiệu lực."}}, "report.json": []any{}}, sourceFetch.SHA256)
	targetFetch := f.addArtifact(t, "fetch", "vn:vbpl:2", map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": "2", "docNum": "39/2016/TT-NHNN", "agencyName": "Ngân hàng Nhà nước Việt Nam", "issueDate": "2016-12-30"}}, "files.json": []any{}})
	article := "article"
	f.addArtifact(t, "extract", "vn:vbpl:2", map[string]any{"provisions.json": []chunk.Node{
		{NodeID: article, Kind: "dieu", Ordinal: 1, Path: "dieu-8", Text: "Điều 8."},
		{NodeID: "clause", ParentNodeID: &article, Kind: "khoan", Ordinal: 1, Path: "dieu-8/khoan-9", Text: "Khoản 9."},
	}, "report.json": []any{}}, targetFetch.SHA256)
	src := "source"
	rows := []relationRow{
		{Ordinal: 0, Kind: "suspends", Method: "text_rule/1", RawCode: "text", TargetID: "2", TargetNumber: "39/2016/TT-NHNN", SourceNode: &src, TargetAnchor: "dieu-8/khoan-9", IssuerCode: "NHNN"},
		{Ordinal: 1, Kind: "suspends", Method: "text_rule/1", RawCode: "text", TargetID: "2", TargetNumber: "39/2016/TT-NHNN", SourceNode: &src, TargetAnchor: "dieu-8/khoan-10", IssuerCode: "NHNN"},
	}
	art := f.addArtifact(t, "relate", "vn:vbpl:1", map[string]any{"relations.json": rows}, f.relateInput(t, "vn:vbpl:1", sourceFetch.SHA256))
	if _, err := Load(context.Background(), f.corpus, f.inventory(t), f.artifacts, f.recipes, nil, nil, f.asOf(), LoadOptions{CentralIssuers: map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"}}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for ordinal, want := range []string{"clause", "article"} {
		var node, requested, source string
		err := f.corpus.QueryRow(ctx, `SELECT p.node_id,r.target_node_id,s.node_id FROM relation r JOIN provision p ON p.id=r.target_provision_id JOIN provision s ON s.id=r.source_provision_id WHERE r.ordinal=$1`, ordinal).Scan(&node, &requested, &source)
		if err != nil || node != want || requested != rows[ordinal].TargetAnchor || source != "source" {
			t.Fatalf("ordinal=%d node=%s requested=%s source=%s err=%v", ordinal, node, requested, source, err)
		}
	}
	var count int
	if err := f.corpus.QueryRow(ctx, `SELECT count(*) FROM coverage_gap WHERE stage='build' AND item_key='vn:vbpl:1' AND kind='relation_clause_unresolved' AND detail=$1`, fmt.Sprintf("%s:1", art.SHA256)).Scan(&count); err != nil || count != 1 {
		t.Fatalf("fallback gaps=%d err=%v", count, err)
	}
}
