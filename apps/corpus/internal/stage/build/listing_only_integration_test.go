//go:build integration

package build

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoad_ListingOnlyThroughChunkEmbedAndBuild(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	key, docType := "vn:vbpl:1", "\n   Văn bản hệ thống hóa  "
	nodes := []chunk.Node{{NodeID: "article", Kind: "dieu", Ordinal: 1, Path: "dieu-1", Text: "Danh mục các văn bản được hệ thống hóa.\nNội dung phải đọc được đầy đủ."}}
	fa := f.addArtifact(t, "fetch", key, map[string]any{
		"detail.json": map[string]any{"data": map[string]any{"id": "1", "docNum": "01/2024/TT", "docType": map[string]any{"name": docType}, "effFrom": "2024-01-01", "effStatus": map[string]any{"code": "CHL"}}},
		"files.json":  []any{},
	})
	ea := f.addArtifact(t, "extract", key, map[string]any{"provisions.json": nodes}, fa.SHA256)
	f.fetch(t, "vn:vbpl:2")
	f.addArtifact(t, "relate", key, map[string]any{"relations.json": []map[string]any{{"ordinal": 0, "kind": "references", "method": "vbpl_references", "target_vbpl_id": "2"}}}, f.relateInput(t, key, fa.SHA256))
	workshop, err := pgxpool.New(ctx, f.workshop.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer workshop.Close()
	env := stage.Env{DB: workshop, Artifacts: f.artifacts}
	ca := runListingStage(t, f, env, chunk.New(), key, stage.RecipeHash("chunk-input", "1", key, fa.SHA256, ea.SHA256))
	file, coverage, err := chunk.ReadArtifact(f.artifacts.Abs(ca.Path))
	if err != nil || len(file.Chunks) != 0 || len(coverage.Gaps) != 1 || coverage.Gaps[0].Kind != "listing_only" || len(coverage.Omissions) == 0 {
		t.Fatalf("chunks=%d coverage=%+v err=%v", len(file.Chunks), coverage, err)
	}
	ba := runListingStage(t, f, env, embed.New(nil, f.recipes.Dense), key, ca.SHA256)
	idx, err := embed.ReadArtifact(f.artifacts.Abs(ba.Path))
	if err != nil || len(idx.Rows) != 0 || len(idx.Rejected) != 0 {
		t.Fatalf("embed=%+v err=%v", idx, err)
	}
	assertListingAcceptance(t, f, workshop)
	counts, err := Load(ctx, f.corpus, f.inventory(t), f.artifacts, f.recipes, map[string]source.StatusCode{"CHL": {Class: "in_force", Current: true}}, nil, f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	if counts.Documents != 2 || counts.Provisions != 1 || counts.Chunks != 0 || counts.DenseVectors != 0 || counts.Rejected != 0 || counts.UnavailableArtifacts != 0 || counts.Relations != 1 {
		t.Fatalf("counts=%+v", counts)
	}
	var state, storedType string
	var complete bool
	var omissions, gaps, resolved int
	err = f.corpus.QueryRow(ctx, `SELECT content_state,doc_type,text_complete,
 (SELECT count(*) FROM coverage_gap WHERE document_key=$1 AND kind IN ('no_chunks','chunk_omission')),
 (SELECT count(*) FROM coverage_gap WHERE document_key=$1 AND kind='listing_only' AND detail=$2),
 (SELECT count(*) FROM relation WHERE source_key=$1 AND source_document_id IS NOT NULL AND target_document_id IS NOT NULL)
 FROM document WHERE key=$1`, key, docType).Scan(&state, &storedType, &complete, &omissions, &gaps, &resolved)
	if err != nil || state != "no_chunks" || storedType != docType || !complete || omissions != 0 || gaps != 2 || resolved != 1 {
		t.Fatalf("state=%s type=%q complete=%v omissions=%d gaps=%d resolved=%d err=%v", state, storedType, complete, omissions, gaps, resolved, err)
	}
	assertListingRetrieval(t, f, counts, nodes[0].Text)
}

func runListingStage(t *testing.T, f *loadFixture, env stage.Env, s stage.Stage, key, input string) artifact.Result {
	t.Helper()
	ctx := context.Background()
	if _, err := env.DB.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,$2,$3,'pending',0)`, f.run.ID, s.Name(), key); err != nil {
		t.Fatal(err)
	}
	item := stage.Item{Key: key, InputHash: input}
	out, err := s.Process(ctx, env, f.run, item)
	if err != nil || out.State != "done" || out.Artifact == nil {
		t.Fatalf("%s output=%+v err=%v", s.Name(), out, err)
	}
	rows, err := s.Rows(ctx, env, *out.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := s.Resolve(ctx, env, f.run, item, *out.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := stage.CommitItem(ctx, env.DB, stage.Commit{RunID: f.run.ID, Stage: s.Name(), ItemKey: key, Generation: f.run.Generation, Attempt: 1, RecipeHash: s.Recipe(f.run.Snapshot), Artifact: out.Artifact, ArtifactRows: rows, RunRows: resolution.RunRows, Gaps: resolution.Gaps, State: out.State})
	if err != nil || !applied {
		t.Fatalf("commit %s applied=%v err=%v", s.Name(), applied, err)
	}
	return *out.Artifact
}

func assertListingAcceptance(t *testing.T, f *loadFixture, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	deps := run.Deps{DB: db}
	gaps, err := run.ListGaps(ctx, deps, "vn", f.run.Rel)
	if err != nil || len(gaps) != 2 {
		t.Fatalf("workshop gaps=%+v err=%v", gaps, err)
	}
	for _, gap := range gaps {
		if gap.Kind != "listing_only" || gap.ItemKey != "vn:vbpl:1" || gap.AcceptedAt != nil {
			t.Fatalf("workshop gap=%+v", gap)
		}
	}
	eligible, _, err := run.Publishable(ctx, db, f.run.ID)
	if err != nil || eligible {
		t.Fatalf("unaccepted gaps: eligible=%v err=%v", eligible, err)
	}
	n, err := run.AcceptGaps(ctx, deps, "vn", f.run.Rel, "listing_only", "reviewed listing-only document")
	if err != nil || n != 2 {
		t.Fatalf("accept gaps=%d err=%v", n, err)
	}
	eligible, reasons, err := run.Publishable(ctx, db, f.run.ID)
	if err != nil || !eligible {
		t.Fatalf("accepted gaps: eligible=%v reasons=%v err=%v", eligible, reasons, err)
	}
}

func assertListingRetrieval(t *testing.T, f *loadFixture, counts Counts, text string) {
	t.Helper()
	ctx := context.Background()
	db, err := pgxpool.New(ctx, f.corpus.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now, err := time.Parse("2006-01-02", f.asOf())
	if err != nil {
		t.Fatal(err)
	}
	query, document := embedding.QueryRecipe1(), f.recipes.Dense
	hash := strings.Repeat("a", 64)
	chunkJSON, err := json.Marshal(chunk.Recipe2())
	if err != nil {
		t.Fatal(err)
	}
	release := retrieval.Release{Country: "vn", Rel: f.run.Rel, SchemaVersion: corpusdb.SchemaVersion, DenseRecipe: retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Document: document, DocumentHash: document.Hash(), Query: query, QueryHash: query.Hash()}, ChunkRecipe: chunkJSON, LexicalRecipe: f.recipes.Lexical, RetrievalRecipe: retrieval.Recipe3(), LexicalStats: counts.LexicalStats, SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, EvaluationSummary: json.RawMessage(`{}`), BuiltAt: now}
	threshold := 0.1
	release.RetrievalRecipe.Abstention.Threshold = &threshold
	release.RetrievalRecipe.Abstention.CalibrationSHA256 = hash
	if err = insertRelease(ctx, f.corpus, release); err != nil {
		t.Fatal(err)
	}
	engine, err := retrieval.Open(ctx, db, structureEmbedder{}, retrieval.Options{Now: func() time.Time { return now }, Profile: "evaluation"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Read(ctx, retrieval.ReadRequest{Release: f.run.Rel, Items: []retrieval.ReadItem{{Handle: "vn:vbpl:1"}}, WholeDocument: true, DocumentKey: "vn:vbpl:1"})
	if err != nil || len(result.Items) != 1 || result.Items[0].Document == nil || result.Items[0].Error != "" || !result.Items[0].Complete {
		t.Fatalf("read=%+v err=%v", result, err)
	}
	var readText strings.Builder
	for _, unit := range result.Items[0].Units {
		if unit.Kind == "provision" {
			readText.WriteString(unit.Text)
		}
	}
	if readText.String() != text {
		t.Fatalf("read text=%q, want %q", readText.String(), text)
	}
	status, err := engine.Status(ctx)
	if err != nil || status.Documents != 2 || status.Provisions != 1 || status.Gaps["listing_only"] != 2 || status.Gaps["chunk_omission"] != 0 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	if len(status.DocTypes) != 1 || status.DocTypes[0].Value != "Văn bản hệ thống hóa" || status.DocTypes[0].Count != 1 {
		t.Fatalf("doc types=%+v", status.DocTypes)
	}
	response, err := engine.Search(ctx, retrieval.Request{Query: "Danh mục văn bản", Mode: "exact", Limit: 8})
	if err != nil || len(response.Hits) != 0 || !response.Abstain {
		t.Fatalf("search=%+v err=%v", response, err)
	}
	suite := evaluation.Suite{Map: evaluation.IdentityMap{Version: 2}, Rejections: []string{"r1", "r2", "r3"}, Legacy: []evaluation.Case{
		{ID: "r1", Question: "synthetic query one", ExpectAbstain: true},
		{ID: "r2", Question: "synthetic query two", ExpectAbstain: true},
		{ID: "r3", Question: "synthetic query three", ExpectAbstain: true},
		{ID: "listing", Question: "Danh mục văn bản", ExpectAbstain: true},
	}}
	report, _, err := evaluation.Run(ctx, engine, suite, evaluation.Options{EvaluationDate: retrieval.Date{Year: now.Year(), Month: now.Month(), Day: now.Day()}, Modes: []retrieval.Mode{"exact"}})
	if err != nil || report.Status != "complete" {
		t.Fatalf("evaluation status=%s err=%v", report.Status, err)
	}
}
