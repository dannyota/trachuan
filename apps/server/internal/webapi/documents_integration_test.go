//go:build integration

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
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

type unusedEmbedder struct{}

func (unusedEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (unusedEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	return embedding.Vector{}, fmt.Errorf("document read called embedder")
}

func documentEngine(t *testing.T, longOutline bool) (*retrieval.Engine, map[string]string) {
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, execErr := db.Exec(ctx, sql, args...); execErr != nil {
			t.Fatal(execErr)
		}
	}
	hash := strings.Repeat("a", 64)
	exec(`INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256) VALUES(1,'vn','vn:1','N1','N1','title','law','issuer','primary','vbpl',false,'2099-01-01','','','','','','in_force','source_status',true,false,'indexed',$1)`, hash)
	source := map[string]string{"a": strings.Repeat("Điều kiện áp dụng. ", 70000) + "Kết thúc.", "b": strings.Repeat("Nghĩa vụ cụ thể. ", 70000) + "Kết thúc."}
	count := 2
	if longOutline {
		count = 8
		source = map[string]string{}
	}
	for i := 1; i <= count; i++ {
		anchor := fmt.Sprintf("p%d", i)
		text := "source"
		heading := "heading"
		if !longOutline {
			if i == 1 {
				anchor = "a"
			} else {
				anchor = "b"
			}
			text = source[anchor]
		} else {
			heading = strings.Repeat("đ", 200000)
			source[anchor] = text
		}
		exec(`INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES($1::int,1,$2,'dieu','Điều',$3,$1::int,$1::int,$2::text::ltree,$4,$5)`, i, fmt.Sprintf("p%d", i), heading, text, anchor)
	}
	now := func() time.Time { return time.Date(2099, 11, 10, 0, 0, 0, 0, time.UTC) }
	rel := retrieval.Release{Country: "vn", Rel: "20991110_1", SchemaVersion: corpusdb.SchemaVersion, RetrievalRecipe: retrieval.Recipe1(), LexicalRecipe: lexical.Recipe1(), ChunkRecipe: json.RawMessage(`{}`), EvaluationSummary: json.RawMessage(`{}`), SourceManifestHash: hash, BuildInputHash: hash, BuildRecipeHash: hash, BuiltAt: now(), LexicalStats: lexical.StatsSummary{Version: 1, AvgDL: 1, VocabularySHA256: hash, DFSHA256: hash}, DenseRecipe: retrieval.DenseRecipe{Version: 1, Model: q.Model, Dimensions: q.Dimensions, Document: d, DocumentHash: d.Hash(), Query: q, QueryHash: q.Hash()}}
	engine, err := retrieval.OpenDraft(ctx, db, unusedEmbedder{}, rel, retrieval.Options{Now: now, Profile: "evaluation"})
	if err != nil {
		t.Fatal(err)
	}
	return engine, source
}

func TestDocuments_IsReadWithWebCapsIntegration(t *testing.T) {
	for _, longOutline := range []bool{false, true} {
		t.Run(fmt.Sprint(longOutline), func(t *testing.T) {
			engine, source := documentEngine(t, longOutline)
			h := newHarness(t)
			h.resolver.engine = engine
			req := retrieval.ReadRequest{Release: "20991110_1", Items: []retrieval.ReadItem{{Handle: "vn:1"}}, PageCap: 2_000_000, PartSize: 1_000_000, WholeDocument: true, DocumentKey: "vn:1"}
			next := ""
			joined := map[string]string{}
			sawText := false
			finished := false
			for page := 0; page < 20; page++ {
				path := "/api/v1/documents/vn:1?release=20991110_1"
				if next != "" {
					path += "&next=" + url.QueryEscape(next)
					req.Items = []retrieval.ReadItem{{Next: next}}
				}
				w := webCall(h, "GET", path, "")
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				var got retrieval.ItemResult
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				want, err := engine.Read(t.Context(), req)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want.Items[0]) {
					t.Fatal("route changed read units, parts or next")
				}
				used := 0
				if got.Document != nil {
					if page != 0 || !got.Document.Status.Current {
						t.Fatal("entry missing status or repeated")
					}
					raw, _ := json.Marshal(got.Document)
					used += len(raw)
				} else if page == 0 {
					t.Fatal("entry missing")
				}
				for _, u := range got.Units {
					raw, _ := json.Marshal(u)
					used += len(raw)
					if len(raw) > 1_000_000 {
						t.Fatal("part exceeds cap")
					}
					if u.Kind == "outline" && sawText {
						t.Fatal("outline after text")
					}
					if u.Kind == "provision" {
						sawText = true
						joined[u.Anchor] += u.Text
					}
				}
				if used > 2_000_000 {
					t.Fatal("page exceeds cap")
				}
				if got.Next == "" {
					if !got.Complete || page == 0 {
						t.Fatal("incomplete chain or missing paging")
					}
					finished = true
					break
				}
				if got.Next == next || got.Complete || len(got.Units) == 0 {
					t.Fatal("page did not advance")
				}
				next = got.Next
			}
			if !finished {
				t.Fatal("pages did not finish")
			}
			for anchor, expected := range source {
				actual, ok := joined[anchor]
				if !ok {
					t.Errorf("anchor %q missing; first differing byte offset 0", anchor)
					continue
				}
				if actual != expected {
					offset := 0
					for offset < min(len(actual), len(expected)) && actual[offset] == expected[offset] {
						offset++
					}
					t.Errorf("anchor %q differs at byte offset %d (got %d bytes, want %d)", anchor, offset, len(actual), len(expected))
				}
			}
			for anchor := range joined {
				if _, ok := source[anchor]; !ok {
					t.Errorf("unexpected anchor %q at byte offset 0", anchor)
				}
			}
		})
	}
}
