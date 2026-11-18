//go:build integration

package evidence

import (
	"context"
	"fmt"
	"testing"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type releaseFixture struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	ctx := context.Background()
	url := testpg.NewDatabase(t, "vector", "ltree")
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err = corpusdb.Apply(ctx, conn, 1024); err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &releaseFixture{t: t, ctx: ctx, pool: pool}
}

func (f *releaseFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *releaseFixture) document(id int64, key, number, title, statusClass, textSource, extract string) {
	f.t.Helper()
	var extractArg any
	if extract != "" {
		extractArg = extract
	}
	f.exec(`INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,validity_conflict,is_current,is_consolidated,text_source,text_complete,content_state,fetch_sha256,extract_sha256)
 VALUES($1,'vn',$2,$3,$4,$5,'Thông tư','NGÂN HÀNG NHÀ NƯỚC VIỆT NAM','primary','vbpl','30/12/2016','','','1','x',$6,'source_status',false,$7,false,$8,false,'indexed',$9,$10)`,
		id, key, number, retrieval.NormalizeNumber(number), title, statusClass, statusClass == "in_force" || statusClass == "partial", textSource, hash64("f"), extractArg)
}

func (f *releaseFixture) provision(id, document int64, parent *int64, kind, anchor, text string, ordinal int) {
	f.t.Helper()
	f.exec(`INSERT INTO provision(id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES($1,$2,$3,$4,$5,'','',$6,$10,$7::ltree,$8,$9)`,
		id, document, parent, fmt.Sprintf("n%d", id), kind, ordinal, fmt.Sprintf("p%d", id), text, anchor, int32(id))
}

func (f *releaseFixture) relation(id, sourceDoc, targetDoc int64, sourceProvision, targetProvision, node any, kind, method string) {
	f.t.Helper()
	f.exec(`INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,source_provision_id,target_provision_id,evidence,method,artifact_sha256,ordinal,target_node_id)
 VALUES($1,$2,$3,'vn:src','vn:target','1','2',$4,'text','x',$5,$6,'e',$7,$8,$9,$10)`,
		id, sourceDoc, targetDoc, kind, sourceProvision, targetProvision, method, hash64("a"), int32(id), node)
}

func (f *releaseFixture) gap(id int64, stageName, kind, key string) {
	f.t.Helper()
	f.exec(`INSERT INTO coverage_gap(id,kind,stage,item_key,detail,message,note) VALUES($1,$2,$3,$4,'d','m','')`, id, kind, stageName, key)
}

func TestCrossSignal_AgainstReleaseDatabase(t *testing.T) {
	f := newReleaseFixture(t)
	waived, waivedSum := extractDir(t, `{"waiver":{"key":"vn:ocr","check_sha256":"`+hash64("c")+`"}}`)
	sentence := "Ngưng hiệu lực khoản 9 Điều 8 của Thông tư 39/2016/TT-NHNN ngày 30/12/2016."
	f.document(1, "vn:src", "10/2099/TT-NHNN", "  Padded title", "in_force", "source_tree", "")
	f.document(2, "vn:target", "39/2016/TT-NHNN", "Target", "partial", "source_tree", "")
	f.document(3, "vn:partial", "11/2099/TT-NHNN", "Partial", "partial", "source_tree", "")
	f.document(4, "vn:ocr", "12/2099/TT-NHNN", "Ocr", "in_force", "ocr", waivedSum)
	f.document(5, "vn:base", "13/2099/TT-NHNN", "Base", "in_force", "source_tree", "")
	f.document(6, "vn:blocked", "14/2099/TT-NHNN", "Blocked", "in_force", "source_tree", "")
	f.document(7, "vn:chinhphu:1", "15/2099/NĐ-CP", "Chinh phu", "in_force", "ocr", waivedSum)
	f.exec(`UPDATE document SET source='chinhphu' WHERE id=7`)
	f.provision(10, 1, nil, "dieu", "dieu-1", sentence, 1)
	f.provision(30, 6, nil, "dieu", "dieu-1", sentence, 1)
	f.provision(20, 2, nil, "dieu", "dieu-8", "Điều 8", 8)
	parent := int64(20)
	f.provision(21, 2, &parent, "khoan", "dieu-8/khoan-9", "9. text", 9)
	f.gap(1, "build", "baseline_served", "vn:base")
	f.gap(2, "extract", "ocr_flagged", "vn:ocr")
	f.gap(3, "extract", "needs_ocr", "vn:blocked")

	run := func(d Detector, dirs func(string, string) (string, bool, error)) Observation {
		obs, err := d.Detect(f.ctx, Inputs{Release: f.pool, ArtifactDir: dirs})
		if err != nil {
			t.Fatalf("%s: %v", d.Spec().ID, err)
		}
		return obs
	}
	dirs := func(_, key string) (string, bool, error) { return waived, key != "vn:base", nil }
	issuers := map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM"}
	severities := func() map[string]string {
		obs, err := detectClaims(f.ctx, sqlRelease{f.pool}, issuers)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, fired := range obs.Findings {
			out[fired.Subject.Fields[0]] = string(fired.Severity)
		}
		if obs.ObservedCompletely(Subject{Type: "claim", Fields: []string{"vn:blocked", "dieu-1", "0"}}) {
			t.Fatal("claim in a source with a needs_ocr gap counts as observed")
		}
		return out
	}

	if obs := run(rawFieldDetector{}, nil); len(obs.Findings) != 1 || obs.Findings[0].Discriminator != "title" || obs.NotObservable != 1 {
		t.Fatalf("raw %+v", obs)
	}
	f.relation(1, 1, 3, nil, nil, nil, "amends_supplements", "vbpl_diagram")
	if obs := run(partialDetector{}, nil); len(obs.Findings) != 2 {
		t.Fatalf("a document-level relation silenced a partial finding: %+v", obs.Findings)
	}
	f.relation(2, 4, 2, nil, nil, nil, "amends_supplements", "vbpl_diagram")
	if obs := run(waivedDetector{}, dirs); len(obs.Findings) != 1 || obs.Findings[0].Subject.Fields[0] != "vn:ocr" || obs.NotObservable != 1 || obs.Observed != 1 {
		t.Fatalf("waived %+v", obs)
	}
	if got := severities(); len(got) != 2 || got["vn:src"] != "review" || got["vn:blocked"] != "info" {
		t.Fatalf("claims %v", got)
	}

	f.relation(3, 1, 2, 10, 21, "dieu-8/khoan-9", "suspends", "text_rule/1")
	f.relation(4, 4, 2, nil, nil, "dieu-8", "suspends", "text_rule/1")
	if got := severities(); len(got) != 1 || got["vn:blocked"] != "info" {
		t.Fatalf("claims after relation %v", got)
	}
	if obs := run(partialDetector{}, nil); len(obs.Findings) != 1 || obs.Findings[0].Subject.Fields[0] != "vn:partial" {
		t.Fatalf("partial after relation %+v", obs.Findings)
	}
	if obs := run(waivedDetector{}, dirs); len(obs.Findings) != 0 {
		t.Fatalf("waived with a text relation %+v", obs.Findings)
	}
}
