//go:build integration

package evaluate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type queryFixture struct{}

func (queryFixture) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (queryFixture) Embed(_ context.Context, text string) (embedding.Vector, error) {
	v := make([]float32, embedding.QueryRecipe1().Dimensions)
	index := 0
	if text == "absent" {
		index = 1
	}
	v[index] = 1
	return embedding.Vector{Values: v, TokenCount: 1, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}
func evaluationFixture(t *testing.T) (*pgx.Conn, EvaluationRequest) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testpg.NewDatabase(t, "vector"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(context.Background())) })
	require.NoError(t, schema.Apply(ctx, conn, 1024))
	hash := strings.Repeat("a", 64)
	req := EvaluationRequest{DatabaseRequest: build.DatabaseRequest{Rel: "20990101"}, RecipeHashes: map[string]string{}, InputHashes: map[string]string{}, CaseFileSHA256: hash}
	steps := map[string]any{}
	for _, name := range []string{"snapshot", "parse", "relate", "chunk", "embed"} {
		version := 1
		parameters := map[string]any{}
		switch name {
		case "snapshot":
			version = 2
		case "relate":
			version = 4
		case "chunk":
			parameters = map[string]any{"schema": "chunks/1", "limit": 512, "prefix_format_version": 1}
		}
		steps[name] = map[string]any{"name": name, "version": version, "parameters": parameters}
		req.RecipeHashes[name] = build.RecipeSHA256(steps[name])
		req.InputHashes[name] = hash
	}
	law := retrieval.Recipe3()
	req.RetrievalRecipe = map[string]any{"version": 1, "status": "measured", "dense_depth": law.DenseDepth, "lexical_depth": law.LexicalDepth, "rrf": law.RRF, "abstention": map[string]any{"statistic": "first-hit-best-permitted-cosine", "threshold": .5, "status": "measured", "calibration_sha256": hash}, "source": map[string]any{"release": "vn/20990101", "recipe_version": 3, "calibration_sha256": hash}}
	doc, query := embedding.DocumentRecipe1(), embedding.QueryRecipe1()
	dense := retrieval.DenseRecipe{Version: 1, Model: doc.Model, Dimensions: doc.Dimensions, Document: doc, DocumentHash: doc.Hash(), Query: query, QueryHash: query.Hash()}
	_, err = conn.Exec(ctx, `INSERT INTO release(singleton,kind,rel,schema_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,snapshot_recipe,parse_recipe,relate_recipe,embed_recipe,snapshot_input_hash,parse_input_hash,relate_input_hash,chunk_input_hash,embed_input_hash,framework_count,version_count,control_count,built_at) VALUES(true,'frameworks','20990101',1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$9,$9,$9,3,4,4,now())`, dense, steps["chunk"], lexical.Recipe1(), req.RetrievalRecipe, steps["snapshot"], steps["parse"], steps["relate"], steps["embed"], hash)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO framework(id,code,name,publisher,aliases) VALUES(1,'fw-alpha','Alpha','Invented','{}'),(2,'fw-beta','Beta','Invented','{}'),(3,'fw-gamma','Gamma','Invented','{}')`)
	require.NoError(t, err)
	vector := make([]float32, 1024)
	vector[0] = 1
	encoded, err := json.Marshal(vector)
	require.NoError(t, err)
	for i, v := range []struct{ code, version, access string }{{"fw-alpha", "1", "licensed"}, {"fw-alpha", "2", "licensed"}, {"fw-beta", "1", "free"}, {"fw-gamma", "1", "licensed"}} {
		id := i + 1
		_, err = conn.Exec(ctx, `INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES($1,$2,$3,'en',$4,'invented','2099-01-01',$1)`, id, v.code, v.version, v.access)
		require.NoError(t, err)
		_, err = conn.Exec(ctx, `INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) VALUES($1,$1,1,'A','A','control','Our title','active')`, id)
		require.NoError(t, err)
		_, err = conn.Exec(ctx, `INSERT INTO control_text(control_id,title_original,body,notes,tables) VALUES($1,'Invented publisher title','Invented body','{}','{}')`, id)
		require.NoError(t, err)
		_, err = conn.Exec(ctx, `INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding) VALUES($1,$1,$1,0,'needle',$2,1,$3::vector)`, id, hash, string(encoded))
		require.NoError(t, err)
		_, err = conn.Exec(ctx, `INSERT INTO public_chunk(id,version_id,control_id,content,input_sha256,token_count,embedding) VALUES($1,$1,$1,'public',$2,1,$3::vector)`, id, hash, string(encoded))
		require.NoError(t, err)
		_, err = conn.Exec(ctx, `INSERT INTO body_posting(version_id,chunk_id,token,tf) VALUES($1,$1,'needle',1)`, id)
		require.NoError(t, err)
		_, err = conn.Exec(ctx, `INSERT INTO public_posting(version_id,chunk_id,token,tf) VALUES($1,$1,'public',1)`, id)
		require.NoError(t, err)
		_, err = conn.Exec(ctx, `INSERT INTO source_file(id,version_id,name,role,sha256,size,licence_kind,text_source,text_complete) VALUES($1,$1,'invented','catalog',$2,1,'invented','structured',true)`, id, hash)
		require.NoError(t, err)
		req.Controls = append(req.Controls, Control{Code: v.code, Version: v.version, Citation: "A"})
	}
	_, err = conn.Exec(ctx, `UPDATE release SET versions_sealed=true; INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved) SELECT id,framework_code,version,'A','fw-beta','1','A','related','invented',id,3,true FROM framework_version WHERE access='licensed'`)
	require.NoError(t, err)
	req.Cases = []GoldenCase{{ID: "p", Question: "needle", Code: "fw-beta", Version: "1", Target: "fw:fw-beta:1#A", Access: "free", Adjudication: "accepted"}, {ID: "n", Question: "absent", Negative: true, Adjudication: "accepted"}}
	req.Calibration = &build.CalibrationRecord{Threshold: .5}
	return conn, req
}
func fixtureOpener(ctx context.Context, conn *pgx.Conn, key []byte) (Engine, func(), error) {
	config, err := pgxpool.ParseConfig(conn.Config().ConnString())
	if err != nil {
		return nil, nil, err
	}
	config.ConnConfig = conn.Config().Copy()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	engine, err := serve.Open(ctx, pool, queryFixture{}, serve.Options{CursorKey: key})
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return engine, pool.Close, nil
}
