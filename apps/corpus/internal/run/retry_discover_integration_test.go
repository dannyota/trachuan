//go:build integration

package run

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestStart_RetriesDoneVBPLDiscoverItemWithSourceGap(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	identity := source.CentralListingIdentity("vbpl", "TRUNG_UONG", 1000)
	for _, statement := range []string{
		`INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('discover','vbpl:central','input','recipe','path','hash')`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) SELECT id,'discover','vbpl:central','done',0,(SELECT id FROM artifact) FROM run`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'discover','vbpl:central','source_unavailable','' FROM run`,
	} {
		if _, err := d.DB.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.DB.Exec(ctx, `INSERT INTO listing(run_id,query_key,item_key,payload) SELECT id,$1,'vn:vbpl:1','{}' FROM run`, identity); err != nil {
		t.Fatal(err)
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var state string
	var ref *int64
	var listings, artifacts int
	err := d.DB.QueryRow(ctx, `SELECT state,artifact_id,(SELECT count(*) FROM listing),(SELECT count(*) FROM artifact) FROM run_item WHERE item_key='vbpl:central'`).Scan(&state, &ref, &listings, &artifacts)
	if err != nil || state != "pending" || ref != nil || listings != 0 || artifacts != 1 {
		t.Fatalf("state=%s ref=%v listings=%d artifacts=%d err=%v", state, ref, listings, artifacts, err)
	}
}

func TestStart_RetrySharedIdentityResetsBothNamedItems(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	identity := source.NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", "71/2025/QH15")
	for _, statement := range []string{
		`INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('discover','first','one','recipe','path1','hash1'),('discover','second','two','recipe','path2','hash2')`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) SELECT id,'discover','vbpl:named:71/2025/QH15@2025-01-01','done',0,(SELECT id FROM artifact WHERE item_key='first') FROM run`,
		`INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) SELECT id,'discover','vbpl:named:71/2025/QH15@2025-02-01','done',0,(SELECT id FROM artifact WHERE item_key='second') FROM run`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'discover','vbpl:named:71/2025/QH15@2025-01-01','source_unavailable','' FROM run`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'discover','vbpl:named:71/2025/QH15@2025-02-01','not_found','' FROM run`,
	} {
		if _, err := d.DB.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.DB.Exec(ctx, `INSERT INTO listing(run_id,query_key,item_key,payload) SELECT id,$1,'vn:vbpl:1','{}' FROM run`, identity); err != nil {
		t.Fatal(err)
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var pending, listings, gaps, artifacts int
	err := d.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM run_item WHERE stage='discover' AND state='pending' AND artifact_id IS NULL),(SELECT count(*) FROM listing),(SELECT count(*) FROM gap),(SELECT count(*) FROM artifact)`).Scan(&pending, &listings, &gaps, &artifacts)
	if err != nil || pending != 2 || listings != 0 || gaps != 0 || artifacts != 2 {
		t.Fatalf("pending=%d listings=%d gaps=%d artifacts=%d err=%v", pending, listings, gaps, artifacts, err)
	}
}

func TestStart_RetryKeepsOtherIdentityListings(t *testing.T) {
	d := retryRun(t, "fetch")
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO run_item(run_id,stage,item_key,state,batch) SELECT id,'discover',k,'done',0 FROM run CROSS JOIN unnest(ARRAY['vbpl:sweep','vbpl:keyword:law']) k`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail) SELECT id,'discover','vbpl:sweep','source_unavailable','' FROM run`,
		`INSERT INTO listing(run_id,query_key,item_key,payload) SELECT id,k,'vn:vbpl:1','{}' FROM run CROSS JOIN unnest(ARRAY['vbpl:sweep@123','vbpl:keyword:law@456']) k`,
	} {
		if _, err := d.DB.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := Start(ctx, d, "vn", d.TaskQueue, "", ""); err != nil {
		t.Fatal(err)
	}
	var key, state string
	err := d.DB.QueryRow(ctx, `SELECT query_key,(SELECT state FROM run_item WHERE item_key='vbpl:keyword:law') FROM listing`).Scan(&key, &state)
	if err != nil || key != "vbpl:keyword:law@456" || state != "done" {
		t.Fatalf("key=%s state=%s err=%v", key, state, err)
	}
}
