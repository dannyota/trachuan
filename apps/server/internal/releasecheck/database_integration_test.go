//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type smokeTransport struct{ calls []string }

func (s *smokeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls = append(s.calls, req.URL.Path)
	query := embedding.QueryRecipe1()
	var result any = embedding.Report{ProtocolVersion: 1, RecipeHash: query.Hash(), Recipe: query}
	if req.URL.Path == "/v1/embed" {
		vector := make([]float32, query.Dimensions)
		vector[0] = 1
		result = map[string]any{"recipe_hash": query.Hash(), "dimensions": query.Dimensions, "data": []any{map[string]any{"index": 0, "token_count": 4, "embedding": vector}}}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw))}, nil
}

func TestCheckDatabase_UsesReadOnlyRoleAndRealSearchAndRead(t *testing.T) {
	ctx := context.Background()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	reader := testpg.NewBuilder(t)
	name, runKey, err := testpg.CreateCorpus(ctx, admin, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, runKey) })
	base.Path = "/" + name
	db, err := pgxpool.New(ctx, base.String())
	if err != nil {
		t.Fatal("connect fixture corpus")
	}
	t.Cleanup(db.Close)
	if _, err = db.Exec(ctx, `CREATE EXTENSION vector; CREATE EXTENSION ltree;`); err != nil {
		t.Fatal(err)
	}
	if err = corpusdb.Apply(ctx, db, 1024); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	if _, err = db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES (1,'vn','vn:synthetic','1','1','Quyền và nghĩa vụ','law','synthetic','primary','vbpl',false,'','','','','','in_force','source_status',true,false,'indexed',$1,'source_tree',true)`, hash); err != nil {
		t.Fatal(err)
	}
	query, document := embedding.QueryRecipe1(), embedding.DocumentRecipe1()
	if _, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (1,1,'dieu-1','dieu','Điều 1','Quyền và nghĩa vụ',1,1,'law.article','Quyền và nghĩa vụ','dieu-1')`); err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, query.Dimensions)
	vector[0] = 1
	vectorJSON, err := json.Marshal(vector)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES (1,1,1,0,'','Quyền và nghĩa vụ','[]',$1,4,$2::vector,'{1:1}/1048576'::sparsevec)`, hash, string(vectorJSON)); err != nil {
		t.Fatal(err)
	}
	dense := retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Query: query, QueryHash: query.Hash(), Document: document, DocumentHash: document.Hash()}
	recipe := retrieval.Recipe3()
	threshold := 0.1
	recipe.Abstention.Threshold, recipe.Abstention.CalibrationSHA256 = &threshold, hash
	stats := lexical.StatsSummary{Version: 1, N: 1, TotalTokens: 4, AvgDL: 4, DistinctTerms: 1, OccupiedDimensions: 1, VocabularySHA256: hash, DFSHA256: hash}
	marshal := func(v any) []byte {
		t.Helper()
		raw, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return raw
	}
	if _, err = db.Exec(ctx, `INSERT INTO release VALUES(true,'vn',$1,$2,'v1',$3,'{}',$4,$5,$6,$7,$7,$7,'{}',now())`, strings.TrimPrefix(name, "corpus_vn_"), corpusdb.SchemaVersion, marshal(dense), marshal(lexical.Recipe1()), marshal(recipe), marshal(stats), hash); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "GRANT USAGE ON SCHEMA public TO "+pgx.Identifier{reader.Role}.Sanitize()+"; GRANT SELECT ON ALL TABLES IN SCHEMA public TO "+pgx.Identifier{reader.Role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(strings.Replace(reader.URL, "/postgres", "/"+name, 1))
	if err != nil {
		t.Fatal("parse synthetic reader configuration")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readOnly, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("connect synthetic reader")
	}
	t.Cleanup(readOnly.Close)
	transport := &smokeTransport{}
	client := embedding.NewClient("http://embedder", &http.Client{Transport: transport})
	if err = checkDatabase(ctx, readOnly, name, client); err != nil {
		t.Fatal(err)
	}
	if len(transport.calls) != 2 || transport.calls[0] != "/v1/recipe" || transport.calls[1] != "/v1/embed" {
		t.Fatalf("smoke embedder calls: %v", transport.calls)
	}
	if _, err = readOnly.Exec(ctx, `DELETE FROM document`); err == nil {
		t.Fatal("reader wrote to the corpus")
	}
	if _, err = db.Exec(ctx, `DELETE FROM chunk; DELETE FROM provision; DELETE FROM document;`); err != nil {
		t.Fatal(err)
	}
	if err = checkDatabase(ctx, readOnly, name, client); err == nil {
		t.Fatal("smoke read accepted an empty corpus")
	}
}
