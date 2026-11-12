//go:build integration

package retrieval

import (
	"context"
	"errors"
	"testing"
)

func taxonomyFixture(t *testing.T) *Engine {
	t.Helper()
	db, e, _ := fixture(t)
	_, err := db.Exec(context.Background(), `INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES
 (1,'vn',NULL,1,'1','Root','','phapdien','active','n1',1,3,1),
 (2,'vn',1,2,'1.1','Ngân hàng','','phapdien','active','n1.n2',2,3,1),
 (3,'vn',1,2,'1.2','Empty','','phapdien','active','n1.n3',1,0,0),
 (4,'vn',2,3,'1.1.c1','Chapter','','phapdien','active','n1.n2.n4',1,3,1),
 (5,'vn',2,3,'1.1.c2','Retired','','phapdien','retired','n1.n2.n5',2,3,1),
 (6,'vn',1,2,'1.3','First','','phapdien','active','n1.n6',3,1,1);
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (11,4,2,'phapdien_seed',false,1,'v1'),(12,2,2,'manual',true,1,'v1')`)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestTaxonomy_OnlyServedNodes(t *testing.T) {
	e := taxonomyFixture(t)
	got, err := e.Taxonomy(context.Background(), TaxonomyRequest{Depth: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].Provisions != 3 || len(got.Nodes[0].Children) != 2 || got.Nodes[0].Children[0].Code != "1.1" || len(got.Nodes[0].Children[0].Children) != 1 {
		t.Fatalf("nodes: %+v", got.Nodes)
	}
	for _, code := range []string{"1.2", "1.1.c2"} {
		_, err = e.TaxonomyNode(context.Background(), code)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", code, err)
		}
	}
}
func TestTaxonomy_DepthAndOrder(t *testing.T) {
	e := taxonomyFixture(t)
	for _, depth := range []int{0, 1, 2, 3} {
		got, err := e.Taxonomy(context.Background(), TaxonomyRequest{Depth: depth})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Nodes) != 1 {
			t.Fatal(got)
		}
		if (depth >= 2) != (len(got.Nodes[0].Children) > 0) {
			t.Fatal(got)
		}
	}
}
func TestTaxonomy_QueryFolded(t *testing.T) {
	e := taxonomyFixture(t)
	got, err := e.Taxonomy(context.Background(), TaxonomyRequest{Q: "ngan hang", Depth: 99})
	if err != nil || len(got.Nodes) != 1 || got.Nodes[0].Code != "1.1" {
		t.Fatalf("%+v %v", got, err)
	}
}
func TestTaxonomyNode_TopDocuments(t *testing.T) {
	e := taxonomyFixture(t)
	got, err := e.TaxonomyNode(context.Background(), "1.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Path) != 1 || len(got.Node.Children) != 1 || len(got.Documents) != 1 || got.Documents[0].Key != "vn:1" || got.Documents[0].Provisions != 1 {
		t.Fatalf("%+v", got)
	}
}
func TestTaxonomy_Truncated(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO taxonomy_node(id,country,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (1,'vn',1,'1','node','','custom','active','n1',1,1,1), (2,'vn',1,'2','node','','custom','active','n2',2,1,1);
 INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) SELECT i+2,'vn',1,2,'1.'||i,'node','','custom','active',('n1.n'||(i+2))::ltree,i,1,1 FROM generate_series(1,500) i`)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"", "node"} {
		got, err := e.Taxonomy(ctx, TaxonomyRequest{Q: q, Depth: 2})
		if err != nil || !got.Truncated {
			t.Fatalf("query %q: %v", q, err)
		}
		if q == "" {
			if len(got.Nodes) != 2 || got.Nodes[1].Code != "2" || len(got.Nodes[0].Children) != 498 {
				t.Fatalf("breadth first: %+v", got)
			}
		} else if len(got.Nodes) != 500 {
			t.Fatalf("query nodes: %d", len(got.Nodes))
		}
	}
}
func TestStatus_Facets(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `UPDATE document SET doc_type=CASE id WHEN 1 THEN '  Café ' WHEN 2 THEN $1 ELSE '' END, issuer=CASE id WHEN 3 THEN '' ELSE ' issuer ' END`, "Cafe\u0301")
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.Status(ctx)
	if err != nil || len(got.DocTypes) != 1 || got.DocTypes[0].Value != "Café" || got.DocTypes[0].Count != 2 || len(got.Issuers) != 1 || got.Issuers[0].Count != 2 {
		t.Fatalf("%+v %v", got, err)
	}
}
func TestTaxonomy_Version4(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `ALTER TABLE taxonomy_node DROP COLUMN ordinal, DROP COLUMN provision_count, DROP COLUMN document_count`)
	if err != nil {
		t.Fatal(err)
	}
	e.release.SchemaVersion = 4
	got, err := e.Taxonomy(ctx, TaxonomyRequest{})
	if err != nil || len(got.Nodes) != 0 {
		t.Fatal(got, err)
	}
	_, err = e.TaxonomyNode(ctx, "1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	status, err := e.Status(ctx)
	if err != nil || status.Nodes != 0 {
		t.Fatal(status, err)
	}
}

func TestTaxonomyNode_DocumentLimitAndTies(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (1,'vn',NULL,1,'1','root','','custom','active','n1',1,30,25), (2,'vn',1,2,'1.1','family','','custom','active','n1.n2',1,30,25);
 INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete)
 SELECT i,'vn','vn:'||lpad(i::text,3,'0'),'n','n','title','law','issuer','primary','vbpl',false,'','','','','','in_force','source_status',true,false,'indexed',repeat('a',64),'source_tree',true FROM generate_series(4,27) i;
 INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT i*10+1,i,'p','dieu','p','',1,1,'n1'::ltree,'','p' FROM generate_series(4,27) i;
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) SELECT i*10+1,2,2,'manual',false,1,'v1' FROM generate_series(4,27) i;
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (11,2,2,'manual',false,1,'v1')`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.TaxonomyNode(ctx, "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Documents) != 20 || got.Documents[0].Key != "vn:1" || got.Documents[0].Provisions != 3 || got.Documents[1].Key != "vn:004" || got.Documents[19].Key != "vn:022" {
		t.Fatalf("documents: %+v", got.Documents)
	}
}
