//go:build integration

package retrieval

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestFilter_NodeInheritance(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	load := func() map[int64][]string {
		t.Helper()
		tx, err := e.metadataTx(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		nodes, _, err := e.loadEffectiveNodes(ctx, tx, []int64{11, 12, 13})
		if err != nil {
			t.Fatal(err)
		}
		return nodes
	}
	nodes := load()
	if !reflect.DeepEqual(nodes[11], []string{"1.1.c1"}) || len(nodes[12]) != 0 || len(nodes[13]) != 0 {
		t.Fatal(nodes)
	}
	req := Request{Nodes: []string{"1.1"}}
	if matchesFilter(req, Identity{}, nodes[12], DocumentEntry{}) {
		t.Fatal("excluded family matched")
	}
	if _, err := e.db.Exec(ctx, `DELETE FROM taxonomy_assignment WHERE provision_id=12`); err != nil {
		t.Fatal(err)
	}
	nodes = load()
	if !reflect.DeepEqual(nodes[12], []string{"1.1.c1"}) || !matchesFilter(req, Identity{}, nodes[12], DocumentEntry{}) {
		t.Fatal("clause did not inherit", nodes)
	}
	if _, err := e.db.Exec(ctx, `INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (11,6,6,'manual',false,1,'v1'),(12,2,2,'manual',true,1,'v1')`); err != nil {
		t.Fatal(err)
	}
	nodes = load()
	if !reflect.DeepEqual(nodes[12], []string{"1.3"}) || !reflect.DeepEqual(nodes[11], []string{"1.1.c1", "1.3"}) {
		t.Fatal("other family lost or wrong depth order", nodes)
	}
}

func TestFilter_UnservedCodeInvalidEngine(t *testing.T) {
	e := taxonomyFixture(t)
	for _, code := range []string{"1.2", "1.1.c2", "99"} {
		_, err := e.Search(context.Background(), Request{Query: "ngân hàng", Nodes: []string{code}})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("%s: %v", code, err)
		}
	}
}

func TestSearch_FilterNodesAndIndexes(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	if _, err := e.db.Exec(ctx, `DELETE FROM taxonomy_assignment WHERE provision_id=12`); err != nil {
		t.Fatal(err)
	}
	req := Request{Query: "ngân hàng", Limit: 2, Nodes: []string{"1.1"}, Mode: "exact"}
	var results []Response
	for {
		out, err := e.Search(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, out)
		presented, err := e.Present(ctx, out, DetailCompact)
		if err != nil {
			t.Fatal(err)
		}
		if presented.OutsideFilterCount == nil {
			t.Fatal("outside count missing")
		}
		for _, hits := range [][]PresentedHit{presented.Hits, presented.NonCurrent} {
			for _, hit := range hits {
				if hit.InFilter == nil {
					t.Fatal("missing in_filter")
				}
				for _, code := range hit.Nodes {
					if _, ok := presented.Nodes[code]; !ok {
						t.Fatal("missing hit node", code)
					}
				}
			}
		}
		for _, doc := range presented.Documents {
			for _, code := range doc.Nodes {
				if _, ok := presented.Nodes[code]; !ok {
					t.Fatal("missing document node", code)
				}
			}
		}
		if out.NextCursor == "" {
			break
		}
		req.Cursor = out.NextCursor
	}
	if len(results) < 2 {
		t.Fatal("paging fixture did not page")
	}
}

func TestNodes_DocumentsAndReadCache(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	cache := &memoryCache{}
	e.options.Documents = cache
	e.options.Sequences = cache
	for range 2 {
		docs, err := e.Documents(ctx, []string{"vn:1"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(docs["vn:1"].Nodes, []string{"1.1.c1"}) {
			t.Fatal(docs)
		}
		read, err := e.Read(ctx, ReadRequest{Release: e.release.Rel, Items: []ReadItem{{Handle: "vn:1"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(read.Items) != 1 || read.Items[0].Document == nil || !reflect.DeepEqual(read.Items[0].Document.Nodes, docs["vn:1"].Nodes) || read.Nodes["1.1.c1"].Level != 3 {
			t.Fatal(read)
		}
	}
}

func TestNodes_DocumentCountsAndTopTen(t *testing.T) {
	_, e, _ := fixture(t)
	ctx := context.Background()
	_, err := e.db.Exec(ctx, `INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (100,'vn',NULL,1,'1','root','','custom','active','n100',1,3,1);
 INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count)
 SELECT i+100,'vn',100,2,'1.'||i,'family','','custom','active',('n100.n'||(i+100))::ltree,i,3,1 FROM generate_series(1,12) i;
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version)
 SELECT 11,i+100,i+100,'manual',false,1,'v1' FROM generate_series(1,12) i;
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (12,101,101,'manual',true,1,'v1')`)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := e.Documents(ctx, []string{"vn:1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.10", "1.11", "1.12", "1.2", "1.3", "1.4", "1.5", "1.6", "1.7", "1.8"}
	if !reflect.DeepEqual(docs["vn:1"].Nodes, want) {
		t.Fatal("counts/ties/cap", docs["vn:1"].Nodes)
	}
}

func TestFilter_Version4(t *testing.T) {
	db, e, _ := fixture(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `ALTER TABLE taxonomy_node DROP COLUMN ordinal,DROP COLUMN provision_count,DROP COLUMN document_count`); err != nil {
		t.Fatal(err)
	}
	e.release.SchemaVersion = 4
	if _, err := e.Search(ctx, Request{Query: "ngân hàng", Nodes: []string{"1"}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	out, err := e.Search(ctx, Request{Query: "ngân hàng", Mode: "exact", DocTypes: []string{" law "}})
	if err != nil {
		t.Fatal(err)
	}
	presented, err := e.Present(ctx, out, DetailCompact)
	if err != nil {
		t.Fatal(err)
	}
	if len(presented.Nodes) != 0 || presented.OutsideFilterCount == nil {
		t.Fatal(presented)
	}
	read, err := e.Read(ctx, ReadRequest{Release: e.release.Rel, Items: []ReadItem{{Handle: "vn:1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Nodes) != 0 || len(read.Items[0].Document.Nodes) != 0 {
		t.Fatal(read)
	}
}

func TestHits_EffectiveNodeDepthAndTieOrder(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	_, err := e.db.Exec(ctx, `INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (7,'vn',6,3,'1.3.c1','Second chapter','','custom','active','n1.n6.n7',1,1,1);
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (11,6,6,'manual',false,1,'v1'),(11,7,6,'manual',false,1,'v1')`)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.metadataTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	nodes, refs, err := e.loadEffectiveNodes(ctx, tx, []int64{11, 12, 13})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(nodes[11], []string{"1.1.c1", "1.3.c1", "1.3"}) {
		t.Fatal("depth/tie order", nodes)
	}
	if len(refs) != 3 || refs["1.3.c1"].Name != "Second chapter" || refs["1.3"].Level != 2 {
		t.Fatal("node index", refs)
	}
}
