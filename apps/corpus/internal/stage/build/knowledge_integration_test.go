//go:build integration

package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/knowledge"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/internal/retrieval"
)

const (
	knowledgeNumber = "1/2020/NĐ-CP"
	knowledgeQuote  = "Bãi bỏ Điều 8 của Nghị định số 1."
	knowledgeActing = "Điều 1. Hiệu lực thi hành\nNghị định này có hiệu lực từ ngày 1/7/2024. " + knowledgeQuote

	ruleCBody   = "Nghị định này có hiệu lực từ ngày 1/2/2021."
	ruleCActing = "Điều 1. Hiệu lực thi hành\n" + ruleCBody
)

var knowledgeCodes = map[string]source.StatusCode{"HHL1P": {Class: "partial", Current: true}, "CHL": {Class: "in_force", Current: true}}

func knowledgeDocuments(t *testing.T, f *loadFixture, textRelation bool) {
	t.Helper()
	target := addValidityFixture(t, f, "1", knowledgeNumber, "HHL1P", "2020-01-01", "2020-01-01", "")
	chapter := "chuong-1"
	f.addArtifact(t, "extract", "vn:vbpl:1", map[string]any{"provisions.json": []chunk.Node{
		{NodeID: chapter, Kind: "chuong", Label: "Chương 1", Heading: "Chung", Ordinal: 1, Path: chapter},
		{NodeID: "dieu-8", ParentNodeID: &chapter, Kind: "dieu", Label: "Điều 8", Heading: "Tám", Ordinal: 2, Path: "dieu-8", Text: "Điều 8 đích."},
		{NodeID: "dieu-9", ParentNodeID: &chapter, Kind: "dieu", Label: "Điều 9", Heading: "Chín", Ordinal: 3, Path: "dieu-9", Text: "Điều 9 đích."},
		{NodeID: "dieu-10", ParentNodeID: &chapter, Kind: "dieu", Label: "Điều 10", Heading: "Mười", Ordinal: 4, Path: "dieu-10", Text: "Điều 10 đích."},
	}, "report.json": []any{}}, target)
	acting := addValidityFixture(t, f, "2", "2/2021/NĐ-CP", "CHL", "2021-01-01", "2021-02-01", "")
	f.addArtifact(t, "extract", "vn:vbpl:2", map[string]any{"provisions.json": []chunk.Node{{NodeID: "article", Kind: "dieu", Label: "Điều 1", Heading: "Hiệu lực thi hành", Ordinal: 1, Path: "dieu-1", Text: knowledgeActing}}, "report.json": []any{}}, acting)
	if !textRelation {
		return
	}
	node, anchor := "article", "dieu-9"
	f.addArtifact(t, "relate", "vn:vbpl:2", map[string]any{"relations.json": []relationRow{{Ordinal: 0, Kind: "suspends", Method: "text_rule/1", RawCode: "text", TargetID: "1", TargetNumber: knowledgeNumber, SourceNode: &node, TargetNode: &anchor}}}, f.relateInput(t, "vn:vbpl:2", acting))
}

func knowledgeProvisionYAML(indent, number, text, heading string, ancestors ...string) string {
	list := "[]"
	if len(ancestors) == 2 {
		list = fmt.Sprintf(`[{ anchor: %q, heading_sha256: %q }]`, ancestors[0], knowledge.Hash(ancestors[1]))
	}
	return fmt.Sprintf("\n%[1]snumber_norm: %[2]q\n%[1]stext_source: \"source_tree\"\n%[1]stext_sha256: %[3]q\n%[1]sheading_sha256: %[4]q\n%[1]sancestors: %[5]s\n", indent, retrieval.NormalizeNumber(number), knowledge.Hash(text), knowledge.Hash(heading), list)
}

func knowledgeEffectYAML(id, targetAnchor, kind, quote, targetText, targetHeading string) string {
	return fmt.Sprintf("- id: %q\n  kind: \"provision_effect\"\n  target: { key: \"vn:vbpl:1\", anchor: %q }\n  claim: { acting: { key: \"vn:vbpl:2\", anchor: \"dieu-1\" }, relation_kind: %q }\n  evidence: { quote: %q }\n  depends:\n    contract: 1\n    acting:%s    target:%s  reviewed: { date: \"2099-11-18\", by: \"primary\", method: \"human\" }\n", id, targetAnchor, kind, quote, knowledgeProvisionYAML("      ", "2/2021/NĐ-CP", knowledgeActing, "Hiệu lực thi hành"), knowledgeProvisionYAML("      ", knowledgeNumber, targetText, targetHeading, "chuong-1", "Chung"))
}

func knowledgeRelationYAML(id, kind, quote, actingText string) string {
	return fmt.Sprintf("- id: %q\n  kind: \"relation\"\n  target: { key: \"vn:vbpl:1\" }\n  claim: { acting: { key: \"vn:vbpl:2\" }, relation_kind: %q }\n  evidence: { anchor: \"dieu-1\", quote: %q }\n  depends:\n    contract: 1\n    provision:%s    target_number_norm: %q\n  reviewed: { date: \"2099-11-18\", by: \"owner\", method: \"human\" }\n", id, kind, quote, knowledgeProvisionYAML("      ", "2/2021/NĐ-CP", actingText, "Hiệu lực thi hành"), retrieval.NormalizeNumber(knowledgeNumber))
}

func knowledgeFileYAML() string {
	baseline := strings.Replace(knowledgeEffectYAML("effect-baseline", "dieu-8", "repeals", knowledgeQuote, "Điều 8 đích.", "Tám"), `key: "vn:vbpl:1"`, `key: "vn:vbpl:3"`, 1)
	return knowledgeDecision +
		knowledgeEffectYAML("effect-applied", "dieu-8", "repeals", knowledgeQuote, "Điều 8 đích.", "Tám") +
		baseline +
		knowledgeEffectYAML("effect-redundant", "dieu-9", "suspends", knowledgeQuote, "Điều 9 đích.", "Chín") +
		knowledgeEffectYAML("effect-stale", "dieu-10", "repeals", knowledgeQuote, "Văn bản cũ.", "Mười") +
		knowledgeRelationYAML("relation-applied", "replaces", knowledgeQuote, knowledgeActing)
}

func knowledgeBaselineDocument(t *testing.T, f *loadFixture) {
	t.Helper()
	addValidityFixture(t, f, "3", "3/2022/NĐ-CP", "CHL", "2022-01-01", "2022-01-01", "")
	if _, err := f.workshop.Exec(context.Background(), `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES($1,'fetch','vn:vbpl:3','source_unavailable','')`, f.run.ID); err != nil {
		t.Fatal(err)
	}
}

func ruleCDocuments(t *testing.T, f *loadFixture, diagramRow bool) {
	t.Helper()
	addValidityFixture(t, f, "1", knowledgeNumber, "CHL", "2020-01-01", "2020-01-01", "")
	acting := addValidityFixture(t, f, "2", "2/2021/NĐ-CP", "CHL", "2021-01-01", "2021-02-01", ruleCBody)
	if diagramRow {
		f.addArtifact(t, "relate", "vn:vbpl:2", map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "replaces", "target_vbpl_id": "1"}}}, f.relateInput(t, "vn:vbpl:2", acting))
	}
}

func knowledgeLoad(t *testing.T, f *loadFixture, input KnowledgeInput) (Counts, error) {
	t.Helper()
	return Load(context.Background(), f.corpus, f.inventory(t), f.artifacts, f.recipes, knowledgeCodes, nil, f.asOf(), LoadOptions{Knowledge: input})
}

func knowledgeTable(t *testing.T, f *loadFixture, query string) string {
	t.Helper()
	var out string
	if err := f.corpus.QueryRow(context.Background(), `SELECT coalesce(json_agg(t)::text,'') FROM (`+query+`) t`).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLoad_KnowledgeAppliesEntries(t *testing.T) {
	ctx := context.Background()
	f := fixture(t)
	knowledgeDocuments(t, f, true)
	knowledgeBaselineDocument(t, f)
	input := knowledgeInputFile(t, knowledgeFileYAML())
	counts, err := knowledgeLoad(t, f, input)
	if err != nil {
		t.Fatal(err)
	}
	want := []knowledge.Record{
		{ID: "effect-applied", Kind: knowledge.KindEffect, TargetKey: "vn:vbpl:1", Outcome: knowledge.OutcomeApplied},
		{ID: "effect-baseline", Kind: knowledge.KindEffect, TargetKey: "vn:vbpl:3", Outcome: knowledge.OutcomeStale, Reason: knowledge.ReasonBaseline},
		{ID: "effect-redundant", Kind: knowledge.KindEffect, TargetKey: "vn:vbpl:1", Outcome: knowledge.OutcomeRedundant},
		{ID: "effect-stale", Kind: knowledge.KindEffect, TargetKey: "vn:vbpl:1", Outcome: knowledge.OutcomeStale, Reason: knowledge.ReasonText},
		{ID: "relation-applied", Kind: knowledge.KindRelation, TargetKey: "vn:vbpl:1", Outcome: knowledge.OutcomeApplied},
	}
	if !reflect.DeepEqual(counts.knowledge.Records, want) {
		t.Fatalf("records = %+v", counts.knowledge.Records)
	}
	var kind, rawCode, label, method, evidence, hash, sourceKey, targetKey, sourceNumber, targetNumber, sourceNode, targetNode, targetProvisionNode string
	var ordinal int
	err = f.corpus.QueryRow(ctx, `SELECT r.kind,r.raw_code,r.raw_label,r.method,r.evidence,r.artifact_sha256,r.source_key,r.target_key,r.source_number,r.target_number,r.source_node_id,r.target_node_id,t.node_id,r.ordinal FROM relation r JOIN provision s ON s.id=r.source_provision_id JOIN provision t ON t.id=r.target_provision_id WHERE r.method='reviewed'`).Scan(&kind, &rawCode, &label, &method, &evidence, &hash, &sourceKey, &targetKey, &sourceNumber, &targetNumber, &sourceNode, &targetNode, &targetProvisionNode, &ordinal)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "repeals" || rawCode != "reviewed" || label != "bãi bỏ" || method != "reviewed" || evidence != knowledgeQuote || hash != input.hash || sourceKey != "vn:vbpl:2" || targetKey != "vn:vbpl:1" || sourceNumber != "2/2021/NĐ-CP" || targetNumber != knowledgeNumber || sourceNode != "article" || targetNode != "dieu-8" || targetProvisionNode != "dieu-8" || ordinal != 1 {
		t.Fatalf("effect row: %s %s %s %s %q %s %s %s %s %s %s %s %s %d", kind, rawCode, label, method, evidence, hash, sourceKey, targetKey, sourceNumber, targetNumber, sourceNode, targetNode, targetProvisionNode, ordinal)
	}
	var documentRows int
	err = f.corpus.QueryRow(ctx, `SELECT count(*) FROM relation WHERE method='reviewed' AND kind='replaces' AND raw_label='thay thế' AND source_provision_id IS NULL AND target_provision_id IS NULL AND source_node_id IS NULL AND target_node_id IS NULL AND evidence=$1 AND ordinal=5`, knowledgeQuote).Scan(&documentRows)
	if err != nil || documentRows != 1 {
		t.Fatalf("document rows = %d, %v", documentRows, err)
	}
	var total int64
	if err = f.corpus.QueryRow(ctx, `SELECT count(*) FROM relation`).Scan(&total); err != nil || total != counts.Relations || total != 3 {
		t.Fatalf("relations %d counted %d: %v", total, counts.Relations, err)
	}
	var detail, stage string
	if err = f.corpus.QueryRow(ctx, `SELECT detail,stage FROM coverage_gap WHERE kind='knowledge_stale' AND item_key='vn:vbpl:1'`).Scan(&detail, &stage); err != nil || detail != "effect-stale:text" || stage != "build" {
		t.Fatalf("gap %q %q: %v", detail, stage, err)
	}
	var baselineGaps, baselineStale int
	if err = f.corpus.QueryRow(ctx, `SELECT count(*) FILTER (WHERE kind='baseline_served' AND stage='build'),count(*) FILTER (WHERE kind='knowledge_stale' AND detail='effect-baseline:baseline') FROM coverage_gap WHERE item_key='vn:vbpl:3'`).Scan(&baselineGaps, &baselineStale); err != nil || baselineGaps != 1 || baselineStale != 1 {
		t.Fatalf("baseline gaps=%d stale=%d: %v", baselineGaps, baselineStale, err)
	}
	release, err := knowledge.Check(ctx, knowledge.SQLReader{Q: f.corpus}, input.file)
	if err != nil {
		t.Fatal(err)
	}
	if release.Records[1] != want[1] {
		t.Fatalf("release baseline record = %+v", release.Records[1])
	}
	if !reflect.DeepEqual(release.Records, counts.knowledge.Records) || !reflect.DeepEqual(release.Rows, counts.knowledge.Rows) || release.Err() != nil {
		t.Fatalf("release check differs from build:\n%+v\n%+v", release, counts.knowledge)
	}
}

func TestLoad_KnowledgeExplainsPartialDocument(t *testing.T) {
	ctx := context.Background()
	entry := knowledgeEffectYAML("effect-applied", "dieu-8", "repeals", knowledgeQuote, "Điều 8 đích.", "Tám")
	for _, tc := range []struct {
		name        string
		input       string
		unexplained int
		acting      int
	}{{"without entries", "[]\n", 1, 0}, {"with an effect", entry, 0, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			f := fixture(t)
			knowledgeDocuments(t, f, false)
			counts, err := knowledgeLoad(t, f, knowledgeInputFile(t, tc.input))
			if err != nil {
				t.Fatal(err)
			}
			partial := counts.validity.partialDocuments
			if len(partial) != 1 || len(partial[0].Acting) != tc.acting {
				t.Fatalf("partial = %+v", partial)
			}
			var unexplained int
			if err = f.corpus.QueryRow(ctx, `SELECT count(*) FROM coverage_gap WHERE kind='partial_status_unexplained'`).Scan(&unexplained); err != nil || unexplained != tc.unexplained {
				t.Fatalf("unexplained = %d, %v", unexplained, err)
			}
		})
	}
}

func TestLoad_KnowledgeLeavesValidityUnchanged(t *testing.T) {
	ctx := context.Background()
	plain, reviewed, control := fixture(t), fixture(t), fixture(t)
	ruleCDocuments(t, plain, false)
	ruleCDocuments(t, reviewed, false)
	ruleCDocuments(t, control, true)
	plainCounts, err := knowledgeLoad(t, plain, KnowledgeInput{})
	if err != nil {
		t.Fatal(err)
	}
	reviewedCounts, err := knowledgeLoad(t, reviewed, knowledgeInputFile(t, knowledgeRelationYAML("relation-rule-c", "replaces", ruleCBody, ruleCActing)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = knowledgeLoad(t, control, KnowledgeInput{}); err != nil {
		t.Fatal(err)
	}
	if records := reviewedCounts.knowledge.Records; len(records) != 1 || records[0].Outcome != knowledge.OutcomeApplied {
		t.Fatalf("records = %+v", records)
	}
	for _, tc := range []struct {
		name string
		f    *loadFixture
		want bool
	}{{"without entries", plain, false}, {"with a reviewed row", reviewed, false}, {"with a diagram row", control, true}} {
		var conflict bool
		if err = tc.f.corpus.QueryRow(ctx, `SELECT validity_conflict FROM document WHERE key='vn:vbpl:1'`).Scan(&conflict); err != nil || conflict != tc.want {
			t.Fatalf("%s: conflict = %v, %v", tc.name, conflict, err)
		}
	}
	for _, query := range []string{`SELECT * FROM document ORDER BY id`, `SELECT * FROM validity_decision ORDER BY document_id,ordinal`, `SELECT * FROM provision_validity ORDER BY id`} {
		if a, b := knowledgeTable(t, plain, query), knowledgeTable(t, reviewed, query); a != b {
			t.Errorf("%s:\n%s\n%s", query, a, b)
		}
	}
	plainReview, err := encodeValidityReview(plainCounts.validity.reviews)
	if err != nil {
		t.Fatal(err)
	}
	reviewedReview, err := encodeValidityReview(reviewedCounts.validity.reviews)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plainReview, reviewedReview) {
		t.Errorf("reviews differ:\n%s\n%s", plainReview, reviewedReview)
	}
}

func TestLoad_KnowledgeErrorFailsBuild(t *testing.T) {
	f := fixture(t)
	knowledgeDocuments(t, f, true)
	entry := knowledgeEffectYAML("effect-bad", "dieu-8", "repeals", "câu này không có trong điều", "Điều 8 đích.", "Tám")
	if _, err := knowledgeLoad(t, f, knowledgeInputFile(t, entry)); err == nil || !strings.Contains(err.Error(), "quote") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoad_KnowledgeContradictedByRowFailsBuild(t *testing.T) {
	f := fixture(t)
	knowledgeDocuments(t, f, true)
	entry := knowledgeEffectYAML("effect-bad", "dieu-9", "repeals", knowledgeQuote, "Điều 9 đích.", "Chín")
	if _, err := knowledgeLoad(t, f, knowledgeInputFile(t, entry)); err == nil || !strings.Contains(err.Error(), "suspends") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoad_NoKnowledgeEntriesChangeNothing(t *testing.T) {
	plain, empty := fixture(t), fixture(t)
	knowledgeDocuments(t, plain, true)
	knowledgeDocuments(t, empty, true)
	if _, err := Load(context.Background(), plain.corpus, plain.inventory(t), plain.artifacts, plain.recipes, knowledgeCodes, nil, plain.asOf()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vn-knowledge.yaml")
	if err := os.WriteFile(path, []byte("# none yet\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input, err := ReadKnowledge("vn", path)
	if err != nil {
		t.Fatal(err)
	}
	timings := map[string]int64{}
	if _, err = Load(context.Background(), empty.corpus, empty.inventory(t), empty.artifacts, empty.recipes, knowledgeCodes, nil, empty.asOf(), LoadOptions{Knowledge: input, Durations: timings}); err != nil {
		t.Fatal(err)
	}
	if _, found := timings["load_knowledge"]; found {
		t.Fatal("empty knowledge added a timing to build.json")
	}
	for _, query := range []string{`SELECT * FROM relation ORDER BY id`, `SELECT id,kind,document_key,stage,item_key,detail,message FROM coverage_gap ORDER BY id`, `SELECT id,document_id,parent_id,node_id,anchor,heading,text FROM provision ORDER BY id`} {
		if a, b := knowledgeTable(t, plain, query), knowledgeTable(t, empty, query); a != b {
			t.Errorf("%s:\n%s\n%s", query, a, b)
		}
	}
}

func TestBuild_KnowledgeFilesInArtifactAndVerified(t *testing.T) {
	ctx := context.Background()
	input := knowledgeInputFile(t, knowledgeDecision)
	f := recoverySetupWith(t, func(cfg *Config) { cfg.Knowledge = input })
	dir := f.env.Artifacts.Abs(f.art.Path)
	data, err := os.ReadFile(filepath.Join(dir, knowledgeFileName))
	if err != nil || string(data) != knowledgeDecision {
		t.Fatalf("knowledge.yaml = %q, %v", data, err)
	}
	report, err := os.ReadFile(filepath.Join(dir, knowledgeReportName))
	if err != nil || strings.TrimSpace(string(report)) != "[]" {
		t.Fatalf("report = %q, %v", report, err)
	}
	if _, err = New(f.cfg).Resolve(ctx, f.env, f.run, f.item, f.art); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_NoKnowledgeFilesWithoutEntries(t *testing.T) {
	f := recoverySetup(t)
	dir := f.env.Artifacts.Abs(f.art.Path)
	for _, name := range []string{knowledgeFileName, knowledgeReportName} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
