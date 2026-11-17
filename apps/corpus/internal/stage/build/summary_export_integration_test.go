//go:build integration

package build

import (
	"context"
	"strings"
	"testing"
)

func TestLoadSummaries_FixtureDatabase(t *testing.T) {
	f := fixture(t)
	f.fetch(t, "vn:vbpl:1")
	if _, err := f.load(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := f.corpus.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := []string{
		`DELETE FROM document_summary`,
		`UPDATE document SET text_source='source_tree' WHERE key='vn:vbpl:1'`,
		`INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT 90001,id,'summary-1','dieu','Điều 1','Điều khoản tóm tắt công khai E2E',90001,90001,'summary1','E2E-BODY-MARKER-7Q4','summary-1' FROM document WHERE key='vn:vbpl:1'`,
		`INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT 90002,id,'summary-2','dieu','Điều 2',repeat('x',320),90002,90002,'summary2','body','summary-2' FROM document WHERE key='vn:vbpl:1'`,
		`INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (90001,'vn',NULL,1,'99','Parent','','phapdien','active','test99',1,1,1),(90002,'vn',90001,2,'99.1','Chủ đề tóm tắt E2E','','phapdien','active','test99.test1',1,1,1)`,
		`INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (90001,90002,90002,'phapdien_seed',false,1,'v1')`,
	}
	for _, q := range queries {
		if _, err = tx.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = LoadSummaries(ctx, tx, nil, nil); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err = tx.QueryRow(ctx, `SELECT payload::text FROM document_summary WHERE document_key='vn:vbpl:1'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Điều khoản tóm tắt công khai E2E", "Chủ đề tóm tắt E2E"} {
		if !strings.Contains(payload, s) {
			t.Errorf("missing %q", s)
		}
	}
	for _, s := range []string{strings.Repeat("x", 320), "E2E-BODY-MARKER-7Q4"} {
		if strings.Contains(payload, s) {
			t.Errorf("unexpected %q", s)
		}
	}
}
