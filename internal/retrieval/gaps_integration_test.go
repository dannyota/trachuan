//go:build integration

package retrieval

import (
	"context"
	"testing"
)

func TestGaps_Paged(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `INSERT INTO coverage_gap(id,kind,document_key,stage,item_key,detail,message,note) VALUES(2,'missing_date','vn:1','build','a','','first',''),(5,'missing_content',NULL,'build','b','','second',''),(11,'missing_date','vn:3','build','c','','third','')`)
	ctx := context.Background()
	page, next, err := e.Gaps(ctx, "", 0, 2)
	if err != nil || len(page) != 2 || page[0].Message != "first" || page[1].Message != "second" || page[1].DocumentKey != "" || next != 5 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "", next, 2)
	if err != nil || len(page) != 1 || page[0].Message != "third" || next != 0 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "missing_date", 0, 2)
	if err != nil || len(page) != 2 || page[1].DocumentKey != "vn:3" || next != 0 {
		t.Fatalf("filtered=%+v next=%d error=%v", page, next, err)
	}
	page, next, err = e.Gaps(ctx, "unknown", 0, 2)
	if err != nil || len(page) != 0 || next != 0 {
		t.Fatalf("unknown=%+v next=%d error=%v", page, next, err)
	}
}
func TestGaps_DefaultPageBudget(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `INSERT INTO coverage_gap(id,kind,stage,item_key,detail,message,note) SELECT n,'test','build',n::text,'',n::text,'' FROM generate_series(1,201) n`)
	page, next, err := e.Gaps(context.Background(), "", 0, 0)
	if err != nil || len(page) != 200 || next != 200 {
		t.Fatalf("size=%d next=%d error=%v", len(page), next, err)
	}
	page, next, err = e.Gaps(context.Background(), "", next, 0)
	if err != nil || len(page) != 1 || page[0].Message != "201" || next != 0 {
		t.Fatalf("page=%+v next=%d error=%v", page, next, err)
	}
}
