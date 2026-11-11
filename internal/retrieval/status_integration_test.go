//go:build integration

package retrieval

import (
	"context"
	"testing"
	"time"
)

func TestStatus_Counts(t *testing.T) {
	db, e, _ := fixture(t)
	readExec(t, db, `UPDATE document SET raw_issue_date=CASE id WHEN 1 THEN '01/02/2024' WHEN 2 THEN '2025-03-04' ELSE '2025-02-30' END`)
	readExec(t, db, `INSERT INTO coverage_gap(id,kind,document_key,stage,item_key,detail,message,note) VALUES(2,'missing_date','vn:1','build','a','','missing date',''),(4,'missing_date',NULL,'build','b','','missing date',''),(9,'missing_content','vn:3','build','c','','missing content','')`)
	s, err := e.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Release != "test" || s.Documents != 3 || s.Provisions != 9 || s.Nodes != 0 || s.Gaps["missing_date"] != 2 || s.Gaps["missing_content"] != 1 || s.NewestSourceDate == nil || s.NewestSourceDate.Format(time.DateOnly) != "2025-03-04" {
		t.Fatalf("status=%+v", s)
	}
}
func TestStatus_EmptyCorpus(t *testing.T) {
	db, e := readFixture(t)
	readExec(t, db, `TRUNCATE document CASCADE`)
	s, err := e.Status(context.Background())
	if err != nil || s.Documents != 0 || s.Provisions != 0 || s.Nodes != 0 || len(s.Gaps) != 0 || s.NewestSourceDate != nil {
		t.Fatalf("status=%+v error=%v", s, err)
	}
}
