//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type densePoolTrace struct {
	queries int
}

func (trace *densePoolTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "WITH pool AS MATERIALIZED") {
		trace.queries++
	}
	return ctx
}

func (*densePoolTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestCollect_OneANNPoolMatchesPerGroupQueries(t *testing.T) {
	trace := &densePoolTrace{}
	db, engine, _ := fixtureWithTrace(t, trace)
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,eff_to,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) SELECT id+3,country,key||':pool','50/2024/TT-NHNN','50/2024/TT-NHNN',title,doc_type,issuer,CASE WHEN id<3 THEN 'relation_context' ELSE 'primary' END,source,validity_conflict,CASE WHEN id=3 THEN '2099-01-01'::date ELSE eff_from END,eff_to,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,CASE WHEN id=3 THEN 'not_yet' ELSE status_class END,status_basis,id=1,is_consolidated,content_state,fetch_sha256,text_source,text_complete FROM document WHERE id<=3`,
		`INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT id*10+1,id,NULL,'pool'||id,'dieu','pool','',1,1,('pool'||id)::ltree,'pool','pool' FROM document WHERE id BETWEEN 4 AND 6`,
		`INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) SELECT d.id+2,d.id,d.id*10+1,0,'',c.content,'[]',c.input_sha256,1,c.embedding,c.sparse FROM document d CROSS JOIN chunk c WHERE d.id BETWEEN 4 AND 6 AND c.id=1`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	for rank, id := range []int{6, 1, 7, 8, 2, 4, 5, 3} {
		values := make([]float32, 1024)
		values[0], values[1] = 1, float32(rank)/10
		if _, err := db.Exec(ctx, `UPDATE chunk SET embedding=$1::vector WHERE id=$2`, denseVector(values), id); err != nil {
			t.Fatal(err)
		}
	}
	engine.release.RetrievalRecipe.ANN.PoolMin = 6
	engine.release.RetrievalRecipe.ANN.PoolMultiplier = 1
	depths := Depths{Dense: 3, Lexical: 3}
	for _, tc := range []struct {
		name     string
		asOf     *Date
		named    bool
		notYet   bool
		ids      [2][]int64
		fallback [2]bool
	}{
		{name: "default", ids: [2][]int64{{1, 2}, {8, 4}}, fallback: [2]bool{true, true}},
		{name: "historical", asOf: &Date{2020, time.June, 1}, ids: [2][]int64{{1, 2, 4}, {8}}, fallback: [2]bool{false, true}},
		{name: "named context", named: true, ids: [2][]int64{{6, 1, 2}, {7, 8, 4}}},
		{name: "historical named context", asOf: &Date{2020, time.June, 1}, named: true, ids: [2][]int64{{6, 1, 7}, {8}}, fallback: [2]bool{false, true}},
		{name: "not yet", notYet: true, ids: [2][]int64{{1, 8, 2}, {4}}, fallback: [2]bool{false, true}},
		{name: "not yet named context", named: true, notYet: true, ids: [2][]int64{{6, 1, 8}, {7, 4}}, fallback: [2]bool{false, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := "ngân hàng"
			if tc.named {
				query += " 50/2024/TT-NHNN"
			}
			prepared, err := engine.Prepare(ctx, Request{Query: query, AsOf: tc.asOf})
			if err != nil {
				t.Fatal(err)
			}
			want := perGroupANNReference(t, engine, prepared, depths, tc.notYet)
			trace.queries = 0
			var got Arms
			if tc.notYet {
				got, err = engine.CollectWithNotYetEligible(ctx, prepared, depths)
			} else {
				got, err = engine.Collect(ctx, prepared, depths)
			}
			if err != nil {
				t.Fatal(err)
			}
			if trace.queries != 1 {
				t.Errorf("pool queries = %d, want 1", trace.queries)
			}
			for i, group := range []ArmGroup{got.Primary, got.NonCurrent} {
				var ids []int64
				for _, candidate := range group.RawANN {
					ids = append(ids, candidate.ChunkID)
				}
				if !reflect.DeepEqual(ids, tc.ids[i]) || group.Fallback != tc.fallback[i] {
					t.Errorf("group %d: raw IDs %v, fallback %t; want %v, %t", i, ids, group.Fallback, tc.ids[i], tc.fallback[i])
				}
				if !reflect.DeepEqual(group.RawANN, want[i].RawANN) || !reflect.DeepEqual(group.Dense, want[i].Dense) || group.Fallback != want[i].Fallback {
					t.Errorf("group %d differs from per-group ANN queries", i)
				}
			}
		})
	}
}

func perGroupANNReference(t *testing.T, engine *Engine, prepared Prepared, depths Depths, notYet bool) [2]ArmGroup {
	t.Helper()
	const eligible = `(($3::boolean OR (d.is_current AND NOT d.validity_conflict)) AND (d.eff_from IS NULL OR d.eff_from <= $2::date) AND (d.eff_to IS NULL OR $2::date < d.eff_to)) = $4::boolean`
	const trial = `((($3::boolean OR (d.is_current AND NOT d.validity_conflict)) AND (d.eff_from IS NULL OR d.eff_from <= $2::date) AND (d.eff_to IS NULL OR $2::date < d.eff_to)) OR (NOT $3::boolean AND d.status_class = 'not_yet')) = $4::boolean`
	const scope = ` AND (d.class <> 'relation_context' OR d.number_norm = ANY($6::text[]))`
	ann := `WITH pool AS MATERIALIZED (SELECT id, document_id, embedding <=> $1::vector AS distance FROM chunk ORDER BY embedding <=> $1::vector LIMIT $7) SELECT p.id, 1-p.distance FROM pool p JOIN document d ON d.id=p.document_id WHERE ` + eligible + scope + ` ORDER BY p.distance,p.id LIMIT $5`
	exact := `SELECT c.id, 1-(c.embedding <=> $1::vector) AS cosine FROM chunk c JOIN document d ON d.id=c.document_id WHERE ` + eligible + scope + ` ORDER BY c.embedding <=> $1::vector, c.id LIMIT $5`
	if notYet {
		ann = strings.Replace(ann, eligible, trial, 1)
		exact = strings.Replace(exact, eligible, trial, 1)
	}
	ctx := context.Background()
	tx, err := engine.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	pool, err := annSettings(ctx, tx, depths.Dense, engine.release.RetrievalRecipe.ANN)
	if err != nil {
		t.Fatal(err)
	}
	vec := denseVector(prepared.Vector.Values)
	refs := append([]string{}, prepared.Validation.References...)
	args := []any{vec, engine.searchDate(prepared.Request), prepared.Request.AsOf != nil, true, depths.Dense, refs}
	var groups [2]ArmGroup
	for i := range groups {
		args[3] = i == 0
		groups[i].RawANN, err = denseQuery(ctx, tx, ann, append(args, pool)...)
		if err != nil {
			t.Fatal(err)
		}
		groups[i].Dense = groups[i].RawANN
	}
	if len(groups[0].RawANN) < depths.Dense || len(groups[1].RawANN) < depths.Dense {
		if err = exactSettings(ctx, tx); err != nil {
			t.Fatal(err)
		}
		for i := range groups {
			if len(groups[i].RawANN) < depths.Dense {
				groups[i].Fallback = true
				args[3] = i == 0
				groups[i].Dense, err = denseQuery(ctx, tx, exact, args...)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err = indexSettings(ctx, tx); err != nil {
		t.Fatal(err)
	}
	for i := range groups {
		if err = hydrate(ctx, tx, &groups[i], vec); err != nil {
			t.Fatal(err)
		}
	}
	return groups
}
