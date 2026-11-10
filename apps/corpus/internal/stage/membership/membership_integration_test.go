//go:build integration

package membership

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5"
)

func fixture(t *testing.T) (context.Context, pgx.Tx, int64) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20260101','paused','{}') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return ctx, tx, id
}

func TestMembership_PublishPhaseFrozen(t *testing.T) {
	ctx, tx, id := fixture(t)
	q := store.New(tx)
	entries := []Entry{{DocumentKey: "vn:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}}
	if err := PublishPhase(ctx, q, id, Seeds, entries); err != nil {
		t.Fatal(err)
	}
	if err := PublishPhase(ctx, q, id, Seeds, entries); err != nil {
		t.Fatal(err)
	}
	entries[0].Class = "relation_context"
	if err := PublishPhase(ctx, q, id, Seeds, entries); err == nil {
		t.Fatal("accepted changed frozen snapshot")
	}
	if err := ClearPhase(ctx, q, id, Seeds); err != nil {
		t.Fatal(err)
	}
	if err := PublishPhase(ctx, q, id, Seeds, entries); err != nil {
		t.Fatal(err)
	}
}

func TestMembership_ReadSortedWithHash(t *testing.T) {
	ctx, tx, id := fixture(t)
	q := store.New(tx)
	entries := []Entry{{DocumentKey: "vn:vbpl:2", Source: "vbpl", SourceID: "2", Class: "primary", Disposition: "withdrawn"}, {DocumentKey: "vn:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}}
	if err := PublishPhase(ctx, q, id, Final, entries); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPhase(ctx, q, id, Final)
	if err != nil {
		t.Fatal(err)
	}
	_, hash, err := CanonicalSnapshot(entries)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != hash || len(got.Entries) != 2 || got.Entries[0].DocumentKey != "vn:vbpl:1" || len(got.Active) != 1 || len(got.RetainedRemovals) != 1 {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestScopeMigration_Constraints(t *testing.T) {
	ctx, tx, id := fixture(t)
	if _, err := tx.Exec(ctx, `INSERT INTO run_document(run_id,document_key,country,source,source_id,class,disposition) VALUES ($1,'vn:vbpl:9','vn','vbpl','9','primary','active')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'expand','9','done',0)`, id); err != nil {
		t.Fatal(err)
	}
	tests := []string{
		`INSERT INTO run_document(run_id,document_key,country,source,source_id,class,disposition) VALUES ($1,'vn:vbpl:1','vn','vbpl','1','bad','active')`,
		`INSERT INTO run_document(run_id,document_key,country,source,source_id,class,disposition) VALUES ($1,'vn:vbpl:1','vn','vbpl','1','primary','bad')`,
		`INSERT INTO run_document(run_id,document_key,country,source,source_id,class,disposition) VALUES ($1,'vn:vbpl:1','vn','vbpl','2','primary','active')`,
		`INSERT INTO run_document(run_id,document_key,country,source,source_id,class,disposition) VALUES ($1,'vn:vbpl:1','vn','vbpl','1','primary','source_replaced')`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail,origin) VALUES ($1,'fetch','1','x','x','bad')`,
		`INSERT INTO gap(run_id,stage,item_key,kind,detail,cause) VALUES ($1,'fetch','1','x','x','bad')`,
		`INSERT INTO run_source(run_id,source,state) VALUES ($1,'vbpl','bad')`,
		`INSERT INTO run_seed_observation(run_id,seed_key,item_key,completeness,artifact_hash,observed_targets) VALUES ($1,'vn:vbpl:9','9','bad','hash','[]')`,
		`INSERT INTO run_membership_evidence(run_id,document_key,stage,item_key,rule_kind,rule_value,response_sha256,artifact_hash,evidence_path,support_state) VALUES ($1,'vn:vbpl:9','expand','9','bad','x','h','h','p','current')`,
		`INSERT INTO run_membership_evidence(run_id,document_key,stage,item_key,rule_kind,rule_value,response_sha256,artifact_hash,evidence_path,support_state) VALUES ($1,'vn:vbpl:9','expand','9','agency','x','h','h','p','bad')`,
	}
	for _, query := range tests {
		sp, err := tx.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = sp.Exec(ctx, query, id)
		if err == nil {
			t.Errorf("accepted %s", query)
		}
		if err := sp.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMembership_RecordEvidence(t *testing.T) {
	ctx, tx, id := fixture(t)
	q := store.New(tx)
	entry := Entry{DocumentKey: "vn:vbpl:1", Source: "vbpl", SourceID: "1", Class: "primary", Disposition: "active"}
	if err := UpsertDocument(ctx, q, id, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch) VALUES ($1,'discover','q','done',0)`, id); err != nil {
		t.Fatal(err)
	}
	record := EvidenceRecord{DocumentKey: entry.DocumentKey, Stage: "discover", ItemKey: "q", State: "current", Support: SupportTuple{RuleKind: "agency", RuleValue: "1", Evidence: EvidenceRef{ResponseSHA256: "response", ArtifactHash: "artifact", Path: "response.json"}}}
	if err := RecordEvidence(ctx, q, id, record); err != nil {
		t.Fatal(err)
	}
	if err := RecordEvidence(ctx, q, id, record); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM run_membership_evidence WHERE run_id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("evidence count = %d", count)
	}
	records, err := ReadEvidence(ctx, q, id)
	if err != nil || len(records) != 1 || records[0].State != "current" {
		t.Fatalf("records = %#v, error = %v", records, err)
	}
	entries, err := ReadDocuments(ctx, q, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || len(entries[0].Supports) != 1 || entries[0].Supports[0] != record.Support {
		t.Fatalf("entries = %#v", entries)
	}
}
