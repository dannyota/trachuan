//go:build integration

package run_test

import (
	"context"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/run"
)

func TestStatus_AdoptionCountDoesNotWaitForItemLocks(t *testing.T) {
	f := newAcquisitionFixture(t)
	f.plan("fetch")
	f.batch(f.fetch)
	id := f.acquired("fetch", "vn:vbpl:1").ArtifactID
	f.exec(`DELETE FROM run_item`)
	f.exec(`UPDATE run SET generation=2`)
	f.exec(`INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,message,artifact_id)
	SELECT $1,s,k,state,0,g,message,CASE WHEN has_artifact THEN $2::bigint ELSE NULL END
	FROM (VALUES
	('fetch','adopted','done',2,'acquisition reused',true),
	('expand','adopted','done',2,'acquisition reused',true),
	('fetch','older','done',1,'acquisition reused',true),
	('extract','other stage','done',2,'acquisition reused',true),
	('fetch','gap','gap',2,'acquisition reused',true),
	('fetch','failed','failed',2,'acquisition reused',true),
	('fetch','pending','pending',2,'acquisition reused',true),
	('fetch','fresh','done',2,'',true),
	('fetch','no artifact','done',2,'acquisition reused',false)) v(s,k,state,g,message,has_artifact)`, f.run.ID, id)
	f.exec(`WITH another AS (INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990111_2','running',2,'{}') RETURNING id)
	INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,message,artifact_id) SELECT id,'fetch','adopted','done',0,2,'acquisition reused',$1 FROM another`, id)
	ctx := context.Background()
	tx, err := f.env.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT item_key FROM run_item WHERE run_id=$1 FOR UPDATE`, f.run.ID); err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	report, err := run.Status(readCtx, f.deps(), "vn", f.run.Rel)
	if err != nil {
		t.Fatalf("status waited for item locks: %v", err)
	}
	if report.AdoptedItems != 2 {
		t.Fatalf("adopted=%d, want 2", report.AdoptedItems)
	}
}
