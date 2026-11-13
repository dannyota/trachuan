//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type siblingHydrationTrace struct {
	queries []string
	pids    []uint32
}

func (trace *siblingHydrationTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	trace.queries = append(trace.queries, data.SQL)
	trace.pids = append(trace.pids, conn.PgConn().PID())
	return ctx
}
func (*siblingHydrationTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestCollect_SiblingArticlesInOneTransaction(t *testing.T) {
	trace := &siblingHydrationTrace{}
	db, engine, _ := fixtureWithTrace(t, trace)
	ctx := context.Background()
	statements := []string{
		`INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (14,1,11,'p14','khoan','Khoản 2','',2,4,'d1.p3'::ltree,'sibling text','p14')`,
		`INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) SELECT 100,1,14,9,context_prefix,content,spans,input_sha256,token_count,embedding,sparse FROM chunk WHERE id=2`,
		`INSERT INTO relation (id,source_document_id,source_key,source_number,target_document_id,target_key,target_number,kind,raw_code,raw_label,target_provision_id,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES (1,2,'vn:2','actor',1,'vn:1','n','repeals','','',14,'p14','sibling warning','text_rule/1',repeat('b',64),0)`,
	}
	for _, sql := range statements {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	collect := func() Arms {
		trace.queries, trace.pids = nil, nil
		return collectFixture(t, engine, Request{Query: "ngân hàng", Mode: "exact"}, Depths{50, 50})
	}
	baseline := collect()
	baselineQueries := append([]string(nil), trace.queries...)
	if baseline.Articles != nil {
		t.Fatal("recipe 2 hydrated sibling articles")
	}
	engine.release.RetrievalRecipe.Version, engine.release.RetrievalRecipe.SiblingMerge = 3, "article-clauses/1"
	arms := collect()
	article, ok := arms.Articles[11]
	if !ok || len(article.Descendants) != 4 || len(article.Relations) != 1 || len(article.Warnings) != 1 || !strings.HasSuffix(article.Warnings[0].Target, "#p14") {
		t.Fatalf("article metadata %+v", article)
	}
	begins, commits := 0, 0
	for i, sql := range trace.queries {
		if strings.HasPrefix(sql, "begin") {
			begins++
			if sql != "begin read only" {
				t.Fatalf("transaction %s", sql)
			}
		}
		if sql == "commit" {
			commits++
		}
		if trace.pids[i] != trace.pids[0] {
			t.Fatal("hydration changed connection")
		}
	}
	if begins != 1 || commits != 1 || trace.queries[0] != "begin read only" || trace.queries[len(trace.queries)-1] != "commit" {
		t.Fatalf("transactions %d/%d", begins, commits)
	}
	if len(trace.queries) <= len(baselineQueries) {
		t.Fatal("no article hydration queries")
	}
	if !reflect.DeepEqual(baseline.Primary, arms.Primary) || !reflect.DeepEqual(baseline.NonCurrent, arms.NonCurrent) {
		t.Fatal("hydration changed candidates")
	}
	// Remove the article-owned candidate to exercise merging; Collect still caches
	// attachments for every possible window, including windows with an article veto.
	filtered := ArmGroup{}
	for _, c := range arms.Primary.Dense {
		if c.GroupID == 12 || c.GroupID == 14 {
			filtered.Dense = append(filtered.Dense, c)
		}
	}
	for _, c := range arms.Primary.Members {
		if c.GroupID == 12 || c.GroupID == 14 {
			filtered.Members = append(filtered.Members, c)
		}
	}
	arms.Primary, arms.NonCurrent = filtered, ArmGroup{}
	response, err := Shape(arms, engine.release.RetrievalRecipe, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Hits) != 1 || response.Hits[0].Identity != article.Identity || len(response.Hits[0].Warnings) != 1 || len(response.Hits[0].Relations) != 1 {
		t.Fatalf("merged response %+v", response)
	}
	engine.release.RetrievalRecipe.Version, engine.release.RetrievalRecipe.SiblingMerge = 2, ""
	again := collect()
	before, _ := json.Marshal(baseline)
	after, _ := json.Marshal(again)
	if string(before) != string(after) || !reflect.DeepEqual(baselineQueries, trace.queries) {
		t.Fatal("recipe 2 output or queries changed")
	}
}
