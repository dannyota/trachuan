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

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

type loadFixture struct {
	workshop, corpus *pgx.Conn
	artifacts        artifact.Store
	run              stage.Run
	recipes          Recipes
}

func fixture(t *testing.T) *loadFixture {
	t.Helper()
	ctx := context.Background()
	w, err := pgx.Connect(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close(ctx) })
	c, err := pgx.Connect(ctx, testpg.NewDatabase(t, "vector", "ltree"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })
	if err = corpusdb.ApplyTables(ctx, c, 1024); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = w.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES('vn','20261109','paused','{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return &loadFixture{workshop: w, corpus: c, artifacts: artifact.Store{Root: t.TempDir()}, run: stage.Run{ID: id, Country: "vn", Rel: "20261109", Snapshot: stage.Snapshot{SourceListHash: strings.Repeat("a", 64)}}, recipes: Recipes{Dense: embedding.DocumentRecipe1(), Lexical: lexical.Recipe1()}}
}
func (f *loadFixture) addArtifact(t *testing.T, name, key string, files map[string]any, inputs ...string) artifact.Result {
	t.Helper()
	ctx := context.Background()
	input := fmt.Sprintf("%x", sha256.Sum256([]byte(name+key)))
	if len(inputs) > 0 {
		input = inputs[0]
	}
	recipe := strings.Repeat("a", 64)
	art, err := f.artifacts.Publish(ctx, name, input, recipe, func(dir string) error {
		for file, value := range files {
			data, raw := value.([]byte)
			var e error
			if !raw {
				data, e = json.Marshal(value)
			}
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(dir, file), data, 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	err = f.workshop.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, name, key, input, recipe, art.Path, art.SHA256).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.workshop.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,input_hash,recipe_hash,state,batch,artifact_id) VALUES($1,$2,$3,$4,$5,'done',0,$6)`, f.run.ID, name, key, input, recipe, id)
	if err != nil {
		t.Fatal(err)
	}
	return art
}
func (f *loadFixture) fetch(t *testing.T, key string) artifact.Result {
	t.Helper()
	return f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": strings.TrimPrefix(key, "vn:vbpl:"), "docNum": "01/2024/TT", "effFrom": "2024-01-01"}}, "files.json": []any{}})
}
func (f *loadFixture) inventory(t *testing.T) Inventory {
	t.Helper()
	inv, err := LoadInventory(context.Background(), store.New(f.workshop), f.run, "embed")
	if err != nil {
		t.Fatal(err)
	}
	return inv
}
func (f *loadFixture) load(t *testing.T) (Counts, error) {
	t.Helper()
	return Load(context.Background(), f.corpus, f.inventory(t), f.artifacts, f.recipes, nil)
}

func TestLoad_IncludesMetadataForNoTree(t *testing.T) {
	f := fixture(t)
	f.fetch(t, "vn:vbpl:1")
	counts, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Documents != 1 || counts.Chunks != 0 {
		t.Fatalf("counts: %+v", counts)
	}
	var state string
	if err = f.corpus.QueryRow(context.Background(), `SELECT content_state FROM document`).Scan(&state); err != nil || state != "no_tree" {
		t.Fatalf("state %q: %v", state, err)
	}
}
func TestLoad_ExcludesWithdrawnAndFetchGaps(t *testing.T) {
	f := fixture(t)
	key := "vn:vbpl:1"
	f.fetch(t, key)
	if _, err := f.workshop.Exec(context.Background(), `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES($1,'fetch',$2,'withdrawn','')`, f.run.ID, key); err != nil {
		t.Fatal(err)
	}
	if _, err := f.workshop.Exec(context.Background(), `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES($1,'fetch','vn:vbpl:2','gap',0)`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.workshop.Exec(context.Background(), `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES($1,'fetch','vn:vbpl:2','missing_source','')`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	counts, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Documents != 0 || counts.Gaps < 2 {
		t.Fatalf("counts: %+v", counts)
	}
}
func TestLoad_RefusesArtifactHashMismatch(t *testing.T) {
	f := fixture(t)
	art := f.fetch(t, "vn:vbpl:1")
	if err := os.WriteFile(filepath.Join(f.artifacts.Abs(art.Path), "detail.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.load(t); err == nil {
		t.Fatal("expected hash failure")
	}
}
func TestLoad_OverLimitFails(t *testing.T) {
	f := fixture(t)
	art := f.fetch(t, "vn:vbpl:1")
	file, err := os.OpenFile(filepath.Join(f.artifacts.Abs(art.Path), "detail.json"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(256<<20 + 1); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = f.load(t); err == nil {
		t.Fatal("expected size failure")
	}
}
func TestLoad_InvalidDatesBecomeNullWithGap(t *testing.T) {
	f := fixture(t)
	key := "vn:vbpl:1"
	f.addArtifact(t, "fetch", key, map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": "1", "effFrom": "2024-02-30"}}, "files.json": []any{}})
	_, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM coverage_gap WHERE kind='invalid_date'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("gap count %d: %v", count, err)
	}
	var missing bool
	if err = f.corpus.QueryRow(context.Background(), `SELECT eff_from IS NULL FROM document`).Scan(&missing); err != nil || !missing {
		t.Fatalf("invalid date was not null: %v", err)
	}
}

func (f *loadFixture) indexed(t *testing.T, key string, rejectMiddle bool, mismatch ...bool) {
	t.Helper()
	fetch := f.fetch(t, key)
	ext := f.addArtifact(t, "extract", key, map[string]any{"provisions.json": []chunk.Node{{NodeID: "a", Kind: "dieu", Ordinal: 1, Path: "dieu-1", Text: "law"}}, "report.json": []any{}}, fetch.SHA256)
	chunks := []chunk.Chunk{{Ordinal: 0, NodeID: "a", Anchor: "dieu-1", Content: "law alpha", Spans: []chunk.Span{}}, {Ordinal: 1, NodeID: "a", Anchor: "dieu-1", Content: "law beta", Spans: []chunk.Span{}}, {Ordinal: 2, NodeID: "a", Anchor: "dieu-1", Content: "law gamma", Spans: []chunk.Span{}}}
	cf := chunk.File{Version: 1, DocumentKey: key, FetchSHA256: fetch.SHA256, ExtractSHA256: ext.SHA256, Chunks: chunks}
	coverage := chunk.Coverage{Version: 1, DocumentKey: key, Gaps: []chunk.Gap{}, Omissions: []chunk.Omission{}, CoveredRanges: []chunk.Range{}, WhitespaceRanges: []chunk.Range{}}
	ca := f.addArtifact(t, "chunk", key, map[string]any{"chunks.json": cf, "coverage.json": coverage}, stage.RecipeHash("chunk-input", "1", key, fetch.SHA256, ext.SHA256))
	idx := embed.Index{Version: 1, DocumentKey: key, ChunkSHA256: ca.SHA256, RecipeHash: f.recipes.Dense.Hash(), Recipe: f.recipes.Dense, Dimensions: 1024, Rows: []embed.Row{}, Rejected: []embed.Rejected{}}
	var vectors []byte
	for _, c := range chunks {
		if rejectMiddle && c.Ordinal == 1 {
			idx.Rejected = append(idx.Rejected, embed.Rejected{ChunkOrdinal: 1, Kind: "embedding_input_bytes_exceeded"})
			continue
		}
		input := "\n" + c.Content
		sum := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))
		idx.Rows = append(idx.Rows, embed.Row{Row: len(idx.Rows), ChunkOrdinal: c.Ordinal, TokenCount: 3, InputSHA256: sum})
		for i := 0; i < 1024; i++ {
			var value float32
			if i == c.Ordinal {
				value = 1
			}
			vectors = binary.LittleEndian.AppendUint32(vectors, math.Float32bits(value))
		}
	}
	if len(mismatch) > 0 && mismatch[0] {
		idx.Rejected = []embed.Rejected{}
	}
	f.addArtifact(t, "embed", key, map[string]any{"index.json": idx, "vectors.f32": vectors, "report.json": map[string]any{"version": 1, "rows": len(idx.Rows), "rejected": len(idx.Rejected), "gaps": []any{}}}, ca.SHA256)
}
func TestLoad_VectorOrdinalAlignment(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", true)
	c, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	if c.Chunks != 2 || c.Rejected != 1 {
		t.Fatalf("counts: %+v", c)
	}
	var value string
	if err = f.corpus.QueryRow(context.Background(), `SELECT embedding::text FROM chunk WHERE ordinal=2`).Scan(&value); err != nil || !strings.HasPrefix(value, "[0,0,1,") {
		t.Fatalf("wrong vector: %v", err)
	}
}
func TestLoad_ChunkCountReconciles(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", true, true)
	if _, err := f.workshop.Exec(context.Background(), `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES($1,'fetch','vn:vbpl:1','withdrawn','')`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.load(t); err == nil {
		t.Fatal("expected chunk count mismatch")
	}
}
func TestLoad_EmbeddingGapDocument(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", true)
	if _, err := f.workshop.Exec(context.Background(), `UPDATE run_item SET state='gap',artifact_id=NULL WHERE stage='embed' AND run_id=$1`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	c, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	var state string
	if err = f.corpus.QueryRow(context.Background(), `SELECT content_state FROM document`).Scan(&state); err != nil || state != "embedding_gap" || c.Chunks != 0 {
		t.Fatalf("state %s, counts %+v, error %v", state, c, err)
	}
}
func TestLoad_LexicalStatsAndDF(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", true)
	c, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	dfJSON := []byte(`[["alpha",1],["gamma",1],["law",2]]`)
	expectedDF := fmt.Sprintf("%x", sha256.Sum256(dfJSON))
	vocab := [][2]any{{"alpha", lexical.Dimension("alpha", f.recipes.Lexical)}, {"gamma", lexical.Dimension("gamma", f.recipes.Lexical)}, {"law", lexical.Dimension("law", f.recipes.Lexical)}}
	vocabularyJSON, err := json.Marshal(vocab)
	if err != nil {
		t.Fatal(err)
	}
	expectedVocab := fmt.Sprintf("%x", sha256.Sum256(vocabularyJSON))
	var df int64
	if err = f.corpus.QueryRow(context.Background(), `SELECT df FROM lexical_df WHERE token='law'`).Scan(&df); err != nil || df != 2 || c.LexicalStats.N != 2 || c.LexicalStats.DFSHA256 != expectedDF || c.LexicalStats.VocabularySHA256 != expectedVocab {
		t.Fatalf("df %d stats %+v: %v", df, c.LexicalStats, err)
	}
}
func TestLoad_UnresolvedRelationsKept(t *testing.T) {
	f := fixture(t)
	key := "vn:vbpl:1"
	fetch := f.fetch(t, key)
	f.addArtifact(t, "relate", key, map[string]any{"relations.json": []relationRow{{Ordinal: 0, Kind: "cites", TargetID: "999", TargetNumber: "99/2024"}}}, fetch.SHA256)
	c, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	var unresolved bool
	if err = f.corpus.QueryRow(context.Background(), `SELECT target_document_id IS NULL AND target_key='vn:vbpl:999' FROM relation`).Scan(&unresolved); err != nil || !unresolved || c.Relations != 1 {
		t.Fatalf("unresolved %v counts %+v: %v", unresolved, c, err)
	}
}

func TestLoad_DoesNotInsertPlaceholderSparseVector(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", false)
	_, err := f.corpus.Exec(context.Background(), `CREATE FUNCTION reject_empty_sparse() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.sparse::text = '{}/1048576' THEN RAISE EXCEPTION 'placeholder sparse vector'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_placeholder BEFORE INSERT ON chunk FOR EACH ROW EXECUTE FUNCTION reject_empty_sparse()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.load(t); err != nil {
		t.Fatalf("load with placeholder guard: %v", err)
	}
}

func TestLoad_IdenticalRelationArtifactsOfTwoDocuments(t *testing.T) {
	f := fixture(t)
	rows := []relationRow{{Ordinal: 0, Kind: "cites", TargetID: "999", TargetNumber: "99/2024"}}
	var hashes []string
	for _, key := range []string{"vn:vbpl:2", "vn:vbpl:1"} {
		fetch := f.fetch(t, key)
		hashes = append(hashes, f.addArtifact(t, "relate", key, map[string]any{"relations.json": rows}, fetch.SHA256).SHA256)
	}
	if hashes[0] != hashes[1] {
		t.Fatalf("fixture needs byte-identical relation artifacts: %v", hashes)
	}
	c, err := f.load(t)
	if err != nil {
		t.Fatal(err)
	}
	var first string
	if err = f.corpus.QueryRow(context.Background(), `SELECT source_key FROM relation WHERE id=1`).Scan(&first); err != nil || first != "vn:vbpl:1" || c.Relations != 2 {
		t.Fatalf("relation 1 source %q counts %+v: %v", first, c, err)
	}
}
