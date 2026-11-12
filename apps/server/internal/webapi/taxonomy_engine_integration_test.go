//go:build integration

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

type taxonomyTexts struct {
	texts []string
	pos   int
}

func (s *taxonomyTexts) Next(context.Context) (string, bool, error) {
	if s.pos == len(s.texts) {
		return "", false, nil
	}
	text := s.texts[s.pos]
	s.pos++
	return text, true, nil
}

type taxonomyEmbedder struct{}

func (taxonomyEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (taxonomyEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	q := embedding.QueryRecipe1()
	values := make([]float32, q.Dimensions)
	values[0] = 1
	return embedding.Vector{Values: values, TokenCount: 1, RecipeHash: q.Hash()}, nil
}

func taxonomyIntegrationEngine(t *testing.T) *retrieval.Engine {
	t.Helper()
	ctx := context.Background()
	q, d := embedding.QueryRecipe1(), embedding.DocumentRecipe1()
	db, err := pgxpool.New(ctx, testpg.NewDatabase(t, "vector", "ltree"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = corpusdb.Apply(ctx, db, q.Dimensions); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	hash := strings.Repeat("a", 64)
	texts := []string{"ngân hàng một", "ngân hàng hai", "ngân hàng ba"}
	stats, err := lexical.Train(ctx, &taxonomyTexts{texts: texts}, lexical.Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	for i, content := range texts {
		id := i + 1
		key := fmt.Sprintf("vn:%d", id)
		exec(`INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES($1,'vn',$2,'n','n','title','law','issuer','primary','vbpl',false,'2099-01-01','','','','','','in_force','source_status',true,false,'indexed',$3,'source_tree',true)`, id, key, hash)
		exec(`INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES($1,$1,'p','dieu','Điều','',1,1,'p',$2,'p')`, id, content)
		sparse, err := lexical.Document(content, stats, lexical.Recipe1())
		if err != nil {
			t.Fatal(err)
		}
		var terms []string
		for _, term := range sparse {
			terms = append(terms, strconv.FormatInt(int64(term.Index), 10)+":"+strconv.FormatFloat(float64(term.Value), 'g', -1, 32))
		}
		sparseText := "{" + strings.Join(terms, ",") + "}/" + strconv.Itoa(lexical.Recipe1().Dimensions)
		vector := "[1" + strings.Repeat(",0", q.Dimensions-1) + "]"
		exec(`INSERT INTO chunk(id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES($1,$1,$1,0,'',$2,'[]',$3,1,$4::vector,$5::sparsevec)`, id, content, hash, vector, sparseText)
	}
	exec(`INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (1,'vn',NULL,1,'1','Root','Root description','custom','active','n1',1,1,1), (2,'vn',1,2,'1.1','Family','Family description','custom','active','n1.n2',1,1,1);
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES(1,2,2,'manual',false,1,'v1')`)
	now := func() time.Time { return time.Date(2099, 11, 10, 0, 0, 0, 0, time.UTC) }
	recipe := retrieval.Recipe1()
	threshold := 0.1
	recipe.Abstention.Threshold = &threshold
	recipe.Abstention.CalibrationSHA256 = hash
	rel := retrieval.Release{Country: "vn", Rel: "20991110_1", SchemaVersion: corpusdb.SchemaVersion, RetrievalRecipe: recipe, LexicalRecipe: lexical.Recipe1(), ChunkRecipe: json.RawMessage(`{}`), EvaluationSummary: json.RawMessage(`{}`), SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, BuiltAt: now(), LexicalStats: stats.Summary(lexical.Recipe1(), nil), DenseRecipe: retrieval.DenseRecipe{Version: 1, Model: q.Model, Dimensions: q.Dimensions, Document: d, DocumentHash: d.Hash(), Query: q, QueryHash: q.Hash()}}
	marshal := func(value any) string {
		t.Helper()
		raw, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return string(raw)
	}
	exec(`INSERT INTO release VALUES (true,'vn',$1,$2,'v1',$3::jsonb,'{}',$4::jsonb,$5::jsonb,$6::jsonb,$7,$7,$7,'{}',$8)`, rel.Rel, rel.SchemaVersion, marshal(rel.DenseRecipe), marshal(rel.LexicalRecipe), marshal(rel.RetrievalRecipe), marshal(rel.LexicalStats), hash, rel.BuiltAt)
	engine, err := retrieval.Open(ctx, db, taxonomyEmbedder{}, retrieval.Options{Now: now, Profile: "evaluation"})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

// wireValue applies the documented omission of empty and null fields to engine output.
func wireValue(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	var clean func(any) any
	clean = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for key, child := range x {
				child = clean(child)
				switch y := child.(type) {
				case nil:
					delete(x, key)
				case string:
					if y == "" {
						delete(x, key)
					}
				case []any:
					if len(y) == 0 {
						delete(x, key)
					}
				case map[string]any:
					if len(y) == 0 {
						delete(x, key)
					}
				}
			}
		case []any:
			for i := range x {
				x[i] = clean(x[i])
			}
		}
		return v
	}
	return clean(decoded)
}

func TestTaxonomyAndFilteredSearch_EngineResultsIntegration(t *testing.T) {
	engine := taxonomyIntegrationEngine(t)
	h := newHarness(t)
	h.resolver.engine = engine
	tree, err := engine.Taxonomy(t.Context(), retrieval.TaxonomyRequest{Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	page, err := engine.TaxonomyNode(t.Context(), "1.1")
	if err != nil {
		t.Fatal(err)
	}
	compare := func(label string, raw []byte, want any) {
		t.Helper()
		var got any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, wireValue(t, want)) {
			t.Fatalf("%s differs from engine: %s", label, raw)
		}
	}
	compare("MCP taxonomy", mcpCall(t, h, "taxonomy", `{"country":"vn","depth":2}`), tree)
	handler := NewPublicTaxonomy(h.webConfig())
	for _, tc := range []struct {
		path, code string
		want       any
	}{{"/api/v1/public/taxonomy?depth=2", "", tree}, {"/api/v1/public/taxonomy/1.1", "1.1", page}} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.SetPathValue("code", tc.code)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		compare(tc.path, w.Body.Bytes(), tc.want)
	}
	req := retrieval.Request{Query: "ngân hàng", Limit: 1, Nodes: []string{"1.1"}}
	response, err := engine.Search(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	want, err := engine.Present(t.Context(), response, retrieval.DetailCompact)
	if err != nil {
		t.Fatal(err)
	}
	if want.OutsideFilterCount == nil || *want.OutsideFilterCount == 0 || want.NextCursor == "" || len(want.Nodes) == 0 || len(want.Hits) != 1 || want.Hits[0].InFilter == nil {
		t.Fatalf("fixture did not exercise filter outputs: %+v", want)
	}
	raw := mcpCall(t, h, "search", `{"country":"vn","query":"ngân hàng","limit":1,"nodes":["1.1"]}`)
	var got retrieval.SearchResult
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.OutsideFilterCount, want.OutsideFilterCount) || got.NextCursor != want.NextCursor || !reflect.DeepEqual(got.Nodes, want.Nodes) || len(got.Hits) != len(want.Hits) || !reflect.DeepEqual(got.Hits[0].InFilter, want.Hits[0].InFilter) || !reflect.DeepEqual(got.Hits[0].Nodes, want.Hits[0].Nodes) {
		t.Fatalf("filtered search differs: %s", raw)
	}
}
