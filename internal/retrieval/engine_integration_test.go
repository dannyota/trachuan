//go:build integration

package retrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationTexts struct {
	texts []string
	pos   int
}

func (it *integrationTexts) Next(context.Context) (string, bool, error) {
	if it.pos == len(it.texts) {
		return "", false, nil
	}
	s := it.texts[it.pos]
	it.pos++
	return s, true, nil
}

type badRecipeEmbedder struct{ fakeEmbedder }

func (f *badRecipeEmbedder) Recipe(context.Context) (embedding.Recipe, error) {
	r := embedding.QueryRecipe1()
	r.Model = "other"
	return r, nil
}

type exactSettingsTrace struct {
	indexScan, bitmapScan string
	pid                   int32
	err                   error
	calls                 int
}

func (trace *exactSettingsTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if data.SQL == exactSQL {
		trace.calls++
		trace.err = conn.QueryRow(ctx, `SELECT current_setting('enable_indexscan'), current_setting('enable_bitmapscan'), pg_backend_pid()`).Scan(&trace.indexScan, &trace.bitmapScan, &trace.pid)
	}
	return ctx
}
func (*exactSettingsTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func fixture(t *testing.T) (*pgxpool.Pool, *Engine, *fakeEmbedder) {
	t.Helper()
	return fixtureWithTrace(t, nil)
}
func fixtureWithTrace(t *testing.T, trace *exactSettingsTrace) (*pgxpool.Pool, *Engine, *fakeEmbedder) {
	t.Helper()
	ctx := context.Background()
	url := testpg.NewDatabase(t, "vector", "ltree")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.RuntimeParams["enable_seqscan"] = "off"
	if trace != nil {
		cfg.ConnConfig.Tracer = trace
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = corpusdb.Apply(ctx, db, 1024); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	for i := 1; i <= 3; i++ {
		status := "in_force"
		start := any("2020-01-01")
		end := any(nil)
		if i == 2 {
			status = "expired"
			end = "2021-01-01"
		}
		if i == 3 {
			start = nil
		}
		_, err = db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,eff_from,eff_to,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,is_current,is_consolidated,content_state,fetch_sha256) VALUES ($1,'vn',$2,'n','n','title','law','issuer',$3,$4,'','','','','',$5,$6,false,'indexed',$7)`, i, fmt.Sprintf("vn:%d", i), start, end, status, i != 2, hash)
		if err != nil {
			t.Fatal(err)
		}
		for j, kind := range []string{"dieu", "khoan", "diem"} {
			id := i*10 + j + 1
			var parent any
			if j > 0 {
				parent = id - 1
			}
			path := fmt.Sprintf("d%d", i)
			for k := 0; k < j; k++ {
				path += fmt.Sprintf(".p%d", k)
			}
			_, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES ($1,$2,$3,$4,$5,$4,'',1,$6,$7::ltree,$4,$4)`, id, i, parent, fmt.Sprintf("p%d", id), kind, j+1, path)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	texts := []string{"ngân hàng điều", "ngân hàng khoản", "tín dụng điểm", "hết hạn", "thiếu ngày"}
	stats, err := lexical.Train(ctx, &integrationTexts{texts: texts}, lexical.Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range texts {
		doc := 1
		prov := 11 + i
		if i == 3 {
			doc = 2
			prov = 23
		}
		if i == 4 {
			doc = 3
			prov = 33
		}
		values := make([]float32, 1024)
		if i == 2 {
			values[1] = 1
		} else {
			values[0] = 1
		}
		var sv lexical.Vector
		sv, err = lexical.Document(s, stats, lexical.Recipe1())
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(ctx, `INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES ($1,$2,$3,$4,'',$5,'[]',$6,1,$7::vector,$8::sparsevec)`, i+1, doc, prov, i, s, hash, denseVector(values), sparseVector(sv, lexical.Recipe1().Dimensions))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(ctx, `UPDATE provision SET text=CASE id WHEN 11 THEN 'ngân hàng điều' ELSE 'ngân hàng khoản' END WHERE id IN (11,12)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE chunk SET spans='[{"node_id":"p11","start":0,"end":4,"content_start":0,"content_end":4,"role":"lead"}]' WHERE id=2`); err != nil {
		t.Fatal(err)
	}

	q, d := embedding.QueryRecipe1(), embedding.DocumentRecipe1()
	dense := DenseRecipe{1, q.Model, q.Dimensions, d, d.Hash(), q, q.Hash()}
	recipe := Recipe1()
	threshold := 0.1
	recipe.Abstention.Threshold = &threshold
	recipe.Abstention.CalibrationSHA256 = hash
	marshal := func(v any) string {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	_, err = db.Exec(ctx, `INSERT INTO release VALUES (true,'vn','test',1,'v1',$1::jsonb,'{}',$2::jsonb,$3::jsonb,$4::jsonb,$5,$5,$5,'{}',now())`, marshal(dense), marshal(lexical.Recipe1()), marshal(recipe), marshal(stats.Summary(lexical.Recipe1(), nil)), hash)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeEmbedder{}
	e, err := Open(ctx, db, f, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}
	return db, e, f
}
func fixtureOptions() Options {
	return Options{Now: func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }, Profile: "evaluation"}
}
func collectFixture(t *testing.T, e *Engine, req Request, depth Depths) Arms {
	t.Helper()
	p, err := e.Prepare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.Collect(context.Background(), p, depth)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func TestOpen_RejectsMissingReleaseFieldsAndTypmodMismatch(t *testing.T) {
	db, _, f := fixture(t)
	ctx := context.Background()
	var original []byte
	if err := db.QueryRow(ctx, `SELECT dense_recipe FROM release`).Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE release SET dense_recipe='{}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, db, f, fixtureOptions()); err == nil {
		t.Fatal("accepted missing dense recipe")
	}
	if _, err := db.Exec(ctx, `UPDATE release SET dense_recipe=$1::jsonb`, original); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `TRUNCATE chunk`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `DROP INDEX chunk_embedding_hnsw`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `ALTER TABLE chunk ALTER COLUMN embedding TYPE vector(1023)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, db, f, fixtureOptions()); err == nil {
		t.Fatal("accepted typmod mismatch")
	}
}
func TestOpen_RejectsEmbedderRecipeMismatch(t *testing.T) {
	db, _, _ := fixture(t)
	if _, err := Open(context.Background(), db, &badRecipeEmbedder{}, fixtureOptions()); err == nil {
		t.Fatal("accepted mismatch")
	}
}
func TestPrepare_EmbedsOnceAndRejectsBeforeEmbedding(t *testing.T) {
	_, e, f := fixture(t)
	ctx := context.Background()
	rejected, err := e.Prepare(ctx, Request{Query: "ngan hang"})
	if err != nil || rejected.Validation.Accepted || f.calls != 0 {
		t.Fatalf("rejected: %+v, calls %d, error %v", rejected, f.calls, err)
	}
	accepted, err := e.Prepare(ctx, Request{Query: "ngân hàng"})
	if err != nil || accepted.Vector == nil || f.calls != 1 {
		t.Fatalf("accepted: %+v, calls %d, error %v", accepted, f.calls, err)
	}
	if _, err = e.Collect(ctx, accepted, Depths{1, 1}); err != nil || f.calls != 1 {
		t.Fatalf("collect: calls %d, error %v", f.calls, err)
	}
}
func TestCollect_ExactDenseOrdersByDistanceThenID(t *testing.T) {
	_, e, _ := fixture(t)
	a := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact"}, Depths{4, 4})
	got := a.Primary.Dense
	if len(got) != 4 || got[0].ChunkID != 1 || got[1].ChunkID != 2 || got[2].ChunkID != 5 || got[3].ChunkID != 3 {
		t.Fatalf("dense: %+v", got)
	}
}
func TestCollect_ExactPlanAvoidsHNSW(t *testing.T) {
	db, _, _ := fixture(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	v := make([]float32, 1024)
	v[0] = 1
	explain := func() string {
		t.Helper()
		rows, queryErr := tx.Query(ctx, "EXPLAIN "+exactSQL, denseVector(v), "2025-01-01", false, true, 4)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		defer rows.Close()
		var lines []string
		for rows.Next() {
			var line string
			if scanErr := rows.Scan(&line); scanErr != nil {
				t.Fatal(scanErr)
			}
			lines = append(lines, line)
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		return strings.Join(lines, "\n")
	}
	control := explain()
	if !strings.Contains(control, "Index Scan using chunk_embedding_hnsw") {
		t.Fatalf("control does not use HNSW index:\n%s", control)
	}
	if err = exactSettings(ctx, tx); err != nil {
		t.Fatal(err)
	}
	exact := explain()
	if strings.Contains(exact, "chunk_embedding_hnsw") {
		t.Fatalf("exact plan uses HNSW index:\n%s", exact)
	}
}
func TestCollect_ANNFallsBackToExactOnShortfall(t *testing.T) {
	_, e, _ := fixture(t)
	a := collectFixture(t, e, Request{Query: "ngân hàng"}, Depths{10, 4})
	if !a.Primary.Fallback || len(a.Primary.RawANN) != 4 || len(a.Primary.Dense) != 4 {
		t.Fatalf("%+v", a.Primary)
	}
	// Raw ANN candidates stay hydrated after the fallback replaces the dense
	// list, so evaluation can report ANN loss by identity.
	for _, c := range a.Primary.RawANN {
		if c.Identity.DocumentKey == "" || c.Identity.Anchor == "" || c.Group.Anchor == "" {
			t.Fatalf("raw ANN candidate %d is not hydrated: %+v", c.ChunkID, c.Identity)
		}
	}
}
func TestCollect_LexicalPositiveScoresOnly(t *testing.T) {
	_, e, _ := fixture(t)
	a := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact"}, Depths{4, 10})
	if len(a.Primary.Lexical) != 2 {
		t.Fatalf("%+v", a.Primary.Lexical)
	}
	for _, c := range a.Primary.Lexical {
		if c.BM25 <= 0 {
			t.Fatal(c)
		}
	}
}
func TestCollect_EligibilityCurrentDefaultAndAsOf(t *testing.T) {
	_, e, _ := fixture(t)
	no := false
	a := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact", InForceOnly: &no}, Depths{10, 10})
	if len(a.Primary.Dense) != 4 || len(a.NonCurrent.Dense) != 1 {
		t.Fatalf("%+v", a)
	}
	a = collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact", AsOf: &Date{2020, time.June, 1}}, Depths{10, 10})
	if len(a.Primary.Dense) != 5 {
		t.Fatalf("%+v", a.Primary.Dense)
	}
}
func TestCollect_ExactSettingsRestoredOnPooledConnection(t *testing.T) {
	trace := &exactSettingsTrace{}
	db, e, _ := fixtureWithTrace(t, trace)
	ctx := context.Background()
	check := func(wantCalls int) {
		t.Helper()
		if trace.calls != wantCalls || trace.err != nil || trace.indexScan != "off" || trace.bitmapScan != "off" {
			t.Fatalf("during exact query: %+v", trace)
		}
		var indexScan, bitmapScan string
		var pid int32
		if err := db.QueryRow(ctx, `SELECT current_setting('enable_indexscan'), current_setting('enable_bitmapscan'), pg_backend_pid()`).Scan(&indexScan, &bitmapScan, &pid); err != nil {
			t.Fatal(err)
		}
		if indexScan != "on" || bitmapScan != "on" || pid != trace.pid {
			t.Fatalf("after collect: index=%s bitmap=%s pid=%d, exact pid=%d", indexScan, bitmapScan, pid, trace.pid)
		}
	}
	collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact"}, Depths{4, 4})
	check(1)
	collectFixture(t, e, Request{Query: "ngân hàng"}, Depths{4, 4})
	check(1)
	if _, err := db.Exec(ctx, `DROP TABLE chunk CASCADE`); err != nil {
		t.Fatal(err)
	}
	p, err := e.Prepare(ctx, Request{Query: "ngân hàng", Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Collect(ctx, p, Depths{4, 4}); err == nil {
		t.Fatal("expected database error")
	}
	check(2)
}
func TestCollect_HydratesUnionWithMemberCosines(t *testing.T) {
	_, e, _ := fixture(t)
	a := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact"}, Depths{1, 2})
	if len(a.Primary.Dense) != 1 || len(a.Primary.Lexical) != 2 {
		t.Fatalf("%+v", a.Primary)
	}
	if !strings.Contains(string(a.Primary.Lexical[1].Spans), `"lead"`) && !strings.Contains(string(a.Primary.Lexical[0].Spans), `"lead"`) {
		t.Fatalf("missing lead spans: %+v", a.Primary.Lexical)
	}
	var member bool
	for _, c := range a.Primary.Members {
		if c.ChunkID == 3 && c.GroupID == 12 && c.Cosine == 0 {
			member = true
			if len(c.Ancestors) != 2 || c.Ancestors[0].ID != 11 || c.Ancestors[1].ID != 12 || c.Provision.ID != 13 {
				t.Fatalf("ancestry: %+v", c)
			}
		}
	}
	if !member {
		t.Fatalf("missing group member: %+v", a.Primary.Members)
	}
}
func TestCollect_NoQueryTextInErrors(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	p, err := e.Prepare(ctx, Request{Query: "ngân hàng secret query", Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `DROP TABLE chunk CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err = e.Collect(ctx, p, Depths{1, 1})
	if err == nil || strings.Contains(err.Error(), p.Request.Query) {
		t.Fatalf("error %v", err)
	}
}

func TestCollect_HydratesDescendantsAndIncidentRelations(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO relation (id,source_document_id,source_key,source_number,target_number,kind,raw_code,raw_label,source_provision_id,evidence,method,artifact_sha256,ordinal) VALUES (1,1,'vn:1','n','other','cites','','',13,'evidence','source',$1,0)`, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	a := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact"}, Depths{1, 2})
	for _, c := range a.Primary.Members {
		if c.GroupID != 12 {
			continue
		}
		if len(c.Descendants) != 2 || c.Descendants[1].ID != 13 || len(c.Relations) != 1 || c.Relations[0].Value.Evidence != "evidence" {
			t.Fatalf("incomplete hydration: %+v", c)
		}
		return
	}
	t.Fatal("missing group")
}

func TestSearch_EndToEndOnSyntheticCorpus(t *testing.T) {
	_, e, _ := fixture(t)
	for _, mode := range []Mode{"exact", "ann"} {
		for _, limit := range []int{8, 20} {
			t.Run(fmt.Sprintf("%s-%d", mode, limit), func(t *testing.T) {
				include := false
				got, err := e.Search(context.Background(), Request{Query: "ngân hàng", Mode: mode, Limit: limit, InForceOnly: &include})
				if err != nil {
					t.Fatal(err)
				}
				if len(got.Hits) == 0 || len(got.NonCurrent) == 0 || got.Abstain {
					t.Fatalf("unexpected search: hits=%d noncurrent=%d abstain=%t", len(got.Hits), len(got.NonCurrent), got.Abstain)
				}
				if got.Hits[0].Identity.DocumentKey != "vn:1" || got.Hits[0].Text == "" {
					t.Fatalf("unexpected first hit: %+v", got.Hits[0].Identity)
				}
			})
		}
	}
}

func TestOpenDraft_SearchRefusedUntilCalibrated(t *testing.T) {
	db, e, f := fixture(t)
	release := e.Release()
	release.RetrievalRecipe.Abstention.Threshold = nil
	release.RetrievalRecipe.Abstention.CalibrationSHA256 = ""
	draft, err := OpenDraft(context.Background(), db, f, release, fixtureOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = draft.Search(context.Background(), Request{Query: "ngân hàng"}); err == nil {
		t.Fatal("draft search accepted")
	}
	if f.calls != 0 {
		t.Fatal("draft search embedded query")
	}
}

func TestSearch_UnknownStatusHistoricalDateIsPrimary(t *testing.T) {
	db, e, _ := fixture(t)
	_, err := db.Exec(context.Background(), `UPDATE document SET status_class='unknown',is_current=false,eff_from='2020-01-01' WHERE id=3`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.Search(context.Background(), Request{Query: "ngân hàng", Mode: "exact", AsOf: &Date{2021, time.January, 1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range got.Hits {
		if hit.Identity.DocumentKey == "vn:3" {
			if !hit.Stamp.Current {
				t.Fatal("historical hit is not current")
			}
			return
		}
	}
	t.Fatal("unknown-status document missing from historical primary hits")
}

func TestCollect_EffectiveToIsFirstExcludedDay(t *testing.T) {
	_, e, _ := fixture(t)
	has := func(a Arms) bool {
		for _, c := range a.Primary.Dense {
			if c.Identity.DocumentKey == "vn:2" {
				return true
			}
		}
		return false
	}
	before := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact", AsOf: &Date{2020, time.December, 31}}, Depths{10, 10})
	on := collectFixture(t, e, Request{Query: "ngân hàng", Mode: "exact", AsOf: &Date{2021, time.January, 1}}, Depths{10, 10})
	if !has(before) || has(on) {
		t.Fatalf("document ending 2021-01-01: eligible on 2020-12-31 %v, on 2021-01-01 %v", has(before), has(on))
	}
}
