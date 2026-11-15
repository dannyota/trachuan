//go:build integration

package retrieval

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestGaps_Paged(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `INSERT INTO coverage_gap(id,kind,document_key,stage,item_key,detail,message,note) VALUES(2,'missing_date','vn:1','build','a','','first',''),(5,'missing_content',NULL,'build','b','','second',''),(11,'missing_date','vn:3','build','c','','third','')`)
	ctx := context.Background()
	page, next, err := e.Gaps(ctx, "", "", 0, 2)
	if err != nil || len(page) != 2 || page[0].Message != "first" || page[1].Message != "second" || page[1].DocumentKey != "" || next != 5 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "", "", next, 2)
	if err != nil || len(page) != 1 || page[0].Message != "third" || next != 0 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "missing_date", "", 0, 2)
	if err != nil || len(page) != 2 || page[1].DocumentKey != "vn:3" || next != 0 {
		t.Fatalf("filtered=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "unknown", "", 0, 2)
	if err != nil || len(page) != 0 || next != 0 {
		t.Fatalf("unknown=%+v next=%d error=%v", page, next, err)
	}
}
func TestGaps_DefaultPageBudget(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `INSERT INTO coverage_gap(id,kind,stage,item_key,detail,message,note) SELECT n,'test','build',n::text,'',n::text,'' FROM generate_series(1,201) n`)
	page, next, err := e.Gaps(context.Background(), "", "", 0, 0)
	if err != nil || len(page) != 200 || next != 200 {
		t.Fatalf("size=%d next=%d error=%v", len(page), next, err)
	}
	page, next, err = e.Gaps(context.Background(), "", "", next, 0)
	if err != nil || len(page) != 1 || page[0].Message != "201" || next != 0 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
}

func TestGaps_NodeFilter(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	_, err := e.db.Exec(ctx, `INSERT INTO coverage_gap(id,kind,document_id,node_id,stage,item_key,detail,message,note) VALUES
 (1,'a',NULL,4,'build','a','','node',''),
 (2,'b',1,NULL,'build','b','','document',''),
 (3,'a',NULL,NULL,'build','c','','global',''),
 (4,'a',NULL,6,'build','d','','outside',''),
 (5,'a',1,6,'build','e','','either','')`)
	if err != nil {
		t.Fatal(err)
	}
	page, next, err := e.Gaps(ctx, "", "1.1", 0, 2)
	if err != nil || len(page) != 2 || page[0].Message != "node" || page[1].Message != "document" || next != 2 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "", "1.1", next, 2)
	if err != nil || len(page) != 1 || page[0].Message != "either" || next != 0 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "a", "1.1", 0, 10)
	if err != nil || len(page) != 2 || page[0].Message != "node" || page[1].Message != "either" || next != 0 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
	for _, code := range []string{"99", "1.2", "bad"} {
		if _, _, err := e.Gaps(ctx, "", code, 0, 2); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("node=%q error=%v", code, err)
		}
	}
}

func TestGaps_SearchNodeParity(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	_, err := e.db.Exec(ctx, `UPDATE taxonomy_node SET parent_id=6,path='n1.n6.n4' WHERE id=4;
 UPDATE taxonomy_assignment SET family_id=6 WHERE provision_id=11 AND node_id=4;
 INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (7,'vn',6,3,'1.3.c1','Inherited','','custom','active','n1.n6.n7',2,3,1);
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (31,4,6,'phapdien_seed',false,1,'v1');
 DELETE FROM taxonomy_assignment WHERE provision_id=31 AND family_id=6;
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (31,7,6,'manual',false,1,'v1');
 INSERT INTO coverage_gap(id,kind,document_id,document_key,stage,item_key,detail,message,note) VALUES (1,'test',1,'vn:1','build','a','','one',''),(2,'test',3,'vn:3','build','b','','three','');
 INSERT INTO coverage_gap(id,kind,node_id,stage,item_key,detail,message,note) VALUES (3,'node',4,'build','c','','moved','')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		code string
		want map[string]bool
		node bool
	}{
		{"1.1", map[string]bool{"vn:1": true}, true},
		{"1.3", map[string]bool{"vn:3": true}, false},
		{"1.3.c1", map[string]bool{"vn:3": true}, false},
	} {
		t.Run(tc.code, func(t *testing.T) {
			searchDocs := map[string]bool{}
			seenDocs := map[string]bool{}
			req := Request{Query: "ngân hàng", Mode: "exact", Arms: "dense", Limit: 20, Nodes: []string{tc.code}}
			for {
				result, err := e.Search(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				for _, hits := range [][]Hit{result.Hits, result.NonCurrent} {
					for _, hit := range hits {
						key := hit.Identity.DocumentKey
						if key != "vn:1" && key != "vn:3" {
							continue
						}
						seenDocs[key] = true
						if hit.InFilter == nil {
							t.Fatal("missing in_filter")
						}
						if *hit.InFilter {
							searchDocs[key] = true
						}
					}
				}
				if result.NextCursor == "" {
					break
				}
				req.Cursor = result.NextCursor
			}
			if !reflect.DeepEqual(seenDocs, map[string]bool{"vn:1": true, "vn:3": true}) || !reflect.DeepEqual(searchDocs, tc.want) {
				t.Fatalf("seen=%v search=%v", seenDocs, searchDocs)
			}
			gaps, _, err := e.Gaps(ctx, "test", tc.code, 0, 10)
			if err != nil {
				t.Fatal(err)
			}
			gapDocs := map[string]bool{}
			for _, gap := range gaps {
				gapDocs[gap.DocumentKey] = true
			}
			if !reflect.DeepEqual(gapDocs, searchDocs) {
				t.Fatalf("gaps=%v search=%v", gapDocs, searchDocs)
			}
			nodes, _, err := e.Gaps(ctx, "node", tc.code, 0, 10)
			if err != nil || (len(nodes) == 1) != tc.node {
				t.Fatalf("nodes=%v error=%v", nodes, err)
			}
		})
	}
}

func TestGaps_ManualOverridesSeed(t *testing.T) {
	e := taxonomyFixture(t)
	ctx := context.Background()
	_, err := e.db.Exec(ctx, `UPDATE taxonomy_node SET parent_id=6,path='n1.n6.n4' WHERE id=4;
 UPDATE taxonomy_assignment SET family_id=6 WHERE provision_id=11 AND node_id=4;
 INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (11,6,6,'manual',false,1,'v1');
 INSERT INTO coverage_gap(id,kind,document_id,document_key,stage,item_key,detail,message,note) VALUES (1,'test',1,'vn:1','build','a','','one','')`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.Search(ctx, Request{Query: "ngân hàng", Mode: "exact", Arms: "dense", Limit: 20, Nodes: []string{"1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, hit := range result.Hits {
		if hit.Identity.DocumentKey == "vn:1" {
			seen = true
			if hit.InFilter == nil || *hit.InFilter {
				t.Fatalf("unexpected search match: %+v", hit)
			}
		}
	}
	if !seen {
		t.Fatal("document missing from search")
	}
	gaps, _, err := e.Gaps(ctx, "test", "1.1", 0, 10)
	if err != nil || len(gaps) != 0 {
		t.Fatalf("gaps=%v error=%v", gaps, err)
	}
}
