//go:build integration

package build

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate/textrule"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoad_ParsedClausesThroughExtractChunkAndBuild(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	key := "vn:vbpl:1"
	var body strings.Builder
	for i := 1; i < 5; i++ {
		fmt.Fprintf(&body, "<p>Điều %d. Mở đầu</p><p>Nội dung chung của văn bản.</p>", i)
	}
	body.WriteString("<p>Điều 5. Quản lý nguồn nước</p><p>1. " + strings.Repeat("Trách nhiệm quản lý nguồn nước. ", 50) + "</p><p>2. Thực hiện</p><p>a) " + strings.Repeat("Bảo vệ tài nguyên. ", 40) + "</p><p>b) " + strings.Repeat("Thanh tra nguồn nước. ", 40) + "</p>")
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(body.String())))
	fa := f.addArtifact(t, "fetch", key, map[string]any{
		"detail.json":   map[string]any{"data": map[string]any{"id": "1", "docNum": "39/2016/TT-NHNN", "agencyName": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", "effFrom": "2020-01-01", "issueDate": "2016-12-30", "effStatus": map[string]any{"code": "CHL", "name": "Còn hiệu lực"}}},
		"tree.json":     map[string]any{"success": true, "data": []any{}},
		"files.json":    []fetch.DocumentFile{{Name: "1_content.html", SHA256: hash, Size: int64(body.Len())}},
		"files/" + hash: []byte(body.String()),
	})
	pool, err := pgxpool.New(ctx, f.workshop.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	env := stage.Env{DB: pool, Artifacts: f.artifacts}
	out, err := extract.New().Process(ctx, env, f.run, stage.Item{Key: key, InputHash: fetch.ExtractInputHash(fa.SHA256, "html_transcription")})
	if err != nil {
		t.Fatal(err)
	}
	if out.Artifact == nil || out.State != "done" {
		t.Fatalf("extract=%+v", out)
	}
	files := map[string]any{}
	for _, name := range []string{"provisions.json", "report.json", "text.json"} {
		data, e := os.ReadFile(filepath.Join(f.artifacts.Abs(out.Artifact.Path), name))
		if e != nil {
			t.Fatal(e)
		}
		files[name] = data
	}
	ea := f.addArtifact(t, "extract", key, files, out.Artifact.InputHash)
	var nodes []chunk.Node
	if err = json.Unmarshal(files["provisions.json"].([]byte), &nodes); err != nil {
		t.Fatal(err)
	}
	cf, coverage, err := chunk.Split(nodes, chunk.Meta{Key: key}, chunk.Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	cf.FetchSHA256, cf.ExtractSHA256 = fa.SHA256, ea.SHA256
	ca := f.addArtifact(t, "chunk", key, map[string]any{"chunks.json": cf, "coverage.json": coverage}, stage.RecipeHash("chunk-input", "1", key, fa.SHA256, ea.SHA256))
	addStructureVectors(t, f, key, ca.SHA256, cf)
	sourceKey := "vn:vbpl:2"
	sourceFetch := f.fetch(t, sourceKey)
	sourceID := "source"
	sourceText := "Bãi bỏ khoản 2 Điều 5 của Thông tư 39/2016/TT-NHNN. Bãi bỏ điểm c khoản 2 Điều 5 của Thông tư 39/2016/TT-NHNN. Bãi bỏ Điều 5 của Thông tư 39/2016/TT-NHNN. Bãi bỏ khoản 1 Điều 5 của Thông tư 39/2016/TT-NHNN. Bãi bỏ khoản 9 Điều 5 của Thông tư 39/2016/TT-NHNN."
	f.addArtifact(t, "extract", sourceKey, map[string]any{"provisions.json": []chunk.Node{{NodeID: sourceID, Kind: "dieu", Ordinal: 1, Path: "dieu-1", Text: sourceText}}}, sourceFetch.SHA256)
	matches, gaps := textrule.Parse(sourceText, map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"})
	if len(matches) != 5 || len(gaps) != 0 {
		t.Fatalf("matches=%+v gaps=%+v", matches, gaps)
	}
	var relations []relationRow
	for i, m := range matches {
		relations = append(relations, relationRow{Ordinal: i, Kind: m.Kind, Method: "text_rule/1", TargetID: "1", TargetNumber: "39/2016/TT-NHNN", SourceNode: &sourceID, TargetAnchor: m.Targets[0].Anchor, IssuerCode: "NHNN", Evidence: sourceText})
	}
	f.addArtifact(t, "relate", sourceKey, map[string]any{"relations.json": relations}, f.relateInput(t, sourceKey, sourceFetch.SHA256))
	counts, err := Load(ctx, f.corpus, f.inventory(t), f.artifacts, f.recipes, map[string]source.StatusCode{"CHL": {Class: "in_force", Current: true}}, nil, f.asOf(), LoadOptions{CentralIssuers: map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"}})
	if err != nil {
		t.Fatal(err)
	}
	if counts.Relations != 5 || counts.Chunks == 0 {
		t.Fatalf("counts=%+v", counts)
	}
	for _, want := range []struct{ anchor, parent string }{{"dieu-5/khoan-1", "dieu-5"}, {"dieu-5/khoan-2", "dieu-5"}, {"dieu-5/khoan-2/diem-a", "dieu-5/khoan-2"}, {"dieu-5/khoan-2/diem-b", "dieu-5/khoan-2"}} {
		var parent string
		var locator *string
		err = f.corpus.QueryRow(ctx, `SELECT parent.anchor,p.locator FROM provision p JOIN provision parent ON parent.id=p.parent_id JOIN document d ON d.id=p.document_id WHERE d.key=$1 AND p.anchor=$2`, key, want.anchor).Scan(&parent, &locator)
		if err != nil || parent != want.parent || locator == nil || !strings.HasPrefix(*locator, "block:") {
			t.Fatalf("%s parent=%s locator=%v: %v", want.anchor, parent, locator, err)
		}
	}
	for _, named := range []string{"dieu-5/khoan-2", "dieu-5/khoan-2/diem-c"} {
		var resolved string
		err = f.corpus.QueryRow(ctx, `SELECT p.anchor FROM relation r JOIN provision p ON p.id=r.target_provision_id WHERE r.target_node_id=$1`, named).Scan(&resolved)
		if err != nil || resolved != "dieu-5/khoan-2" {
			t.Fatalf("%s resolved=%s: %v", named, resolved, err)
		}
	}
	var unresolved int
	err = f.corpus.QueryRow(ctx, `SELECT count(*) FROM coverage_gap WHERE kind='relation_clause_unresolved'`).Scan(&unresolved)
	if err != nil || unresolved != 2 {
		t.Fatalf("unresolved=%d: %v", unresolved, err)
	}
	var articleChunks int
	err = f.corpus.QueryRow(ctx, `SELECT count(*) FROM chunk c JOIN provision p ON p.id=c.provision_id WHERE p.anchor='dieu-5'`).Scan(&articleChunks)
	if err != nil || articleChunks != 0 {
		t.Fatalf("large article chunks=%d: %v", articleChunks, err)
	}
	assertStructureHit(t, f, counts)
}

func addStructureVectors(t *testing.T, f *loadFixture, key, chunkHash string, cf chunk.File) {
	t.Helper()
	idx := embed.Index{Version: 1, DocumentKey: key, ChunkSHA256: chunkHash, RecipeHash: f.recipes.Dense.Hash(), Recipe: f.recipes.Dense, Dimensions: 1024, Rows: []embed.Row{}, Rejected: []embed.Rejected{}}
	var vectors []byte
	for _, c := range cf.Chunks {
		sum := fmt.Sprintf("%x", sha256.Sum256([]byte(c.ContextPrefix+"\n"+c.Content)))
		idx.Rows = append(idx.Rows, embed.Row{Row: len(idx.Rows), ChunkOrdinal: c.Ordinal, TokenCount: 3, InputSHA256: sum})
		for i := 0; i < 1024; i++ {
			var v float32
			if i == 0 {
				v = 1
			}
			vectors = binary.LittleEndian.AppendUint32(vectors, math.Float32bits(v))
		}
	}
	f.addArtifact(t, "embed", key, map[string]any{"index.json": idx, "vectors.f32": vectors, "report.json": map[string]any{"version": 1, "rows": len(idx.Rows), "rejected": 0, "gaps": []any{}}}, chunkHash)
}

type structureEmbedder struct{}

func (structureEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (structureEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	values := make([]float32, 1024)
	values[0] = 1
	return embedding.Vector{Values: values, TokenCount: 1, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}

func assertStructureHit(t *testing.T, f *loadFixture, counts Counts) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, f.corpus.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	query, document := embedding.QueryRecipe1(), f.recipes.Dense
	now, err := time.Parse("2006-01-02", f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	release := retrieval.Release{Country: "vn", Rel: f.run.Rel, SchemaVersion: corpusdb.SchemaVersion, DenseRecipe: retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Document: document, DocumentHash: document.Hash(), Query: query, QueryHash: query.Hash()}, ChunkRecipe: json.RawMessage(`{}`), LexicalRecipe: f.recipes.Lexical, RetrievalRecipe: retrieval.Recipe1(), LexicalStats: counts.LexicalStats, SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, EvaluationSummary: json.RawMessage(`{}`), BuiltAt: now}
	calibration := evaluation.Calibration{Threshold: 0.1}
	calibrationJSON, err := json.Marshal(calibration)
	if err != nil {
		t.Fatal(err)
	}
	release.RetrievalRecipe.Abstention.Threshold = &calibration.Threshold
	release.RetrievalRecipe.Abstention.CalibrationSHA256 = fmt.Sprintf("%x", sha256.Sum256(calibrationJSON))
	if err = insertRelease(ctx, f.corpus, release); err != nil {
		t.Fatal(err)
	}
	engine, err := retrieval.Open(ctx, pool, structureEmbedder{}, retrieval.Options{Now: func() time.Time { return now }, Profile: "evaluation"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := engine.Search(ctx, retrieval.Request{Query: "trách nhiệm quản lý nguồn nước", Mode: "exact", Arms: "lexical", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Present(ctx, response, retrieval.DetailFull)
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range result.Hits {
		if hit.Handle != "vn:vbpl:1#dieu-5/khoan-1" {
			continue
		}
		status := result.Documents[hit.Doc].Status
		if status.Class != "in_force" || !status.Current {
			t.Fatalf("clause status=%+v", status)
		}
		if len(hit.Warnings) != 2 {
			t.Fatalf("clause warnings=%+v", hit.Warnings)
		}
		targets := map[string]bool{}
		for _, warning := range hit.Warnings {
			if !warning.Exact {
				t.Fatalf("inexact ancestor warning: %+v", warning)
			}
			targets[warning.Target] = true
		}
		if !targets["vn:vbpl:1#dieu-5"] || !targets["vn:vbpl:1#dieu-5/khoan-1"] {
			t.Fatalf("clause warning targets=%v", targets)
		}
		return
	}
	t.Fatalf("missing parsed clause-owned hit: %+v", result.Hits)
}
