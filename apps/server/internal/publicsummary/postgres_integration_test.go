//go:build integration

package publicsummary

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgres_SummaryOnlyAndBoundedList(t *testing.T) {
	ctx := context.Background()
	databaseURL := testpg.NewDatabase(t)
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "2000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE document_summary(document_key text COLLATE "C" PRIMARY KEY,indexed boolean,projection integer,status_class text,status_basis text,is_current boolean,validity_conflict boolean,eff_from date,eff_to date,gap_kinds text[],payload jsonb);
 INSERT INTO document_summary VALUES('vn:vbpl:1',true,1,'in_force','source_status',true,false,'2099-01-01',NULL,'{}','{}')`)
	if err != nil {
		t.Fatal(err)
	}
	source, err := OpenPostgres(databaseURL)(ctx, "corpus_vn_20990101")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	p := source.(*postgresPool)
	row, err := p.Row(ctx, "vn:vbpl:1")
	if err != nil || row.Key != "vn:vbpl:1" || !row.Indexed || row.EffectiveFrom == nil || string(row.Payload) != "{}" {
		t.Fatalf("summary: %+v %v", row, err)
	}
	if _, err = p.Row(ctx, "vn:vbpl:missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE document_summary SET payload=$1`, fixturePayload); err != nil {
		t.Fatal(err)
	}
	release := Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1, HeadingsApproved: true}
	clock := func() time.Time { return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC) }
	manager := NewManager(func(context.Context) (Release, error) { return release, nil }, OpenPostgres(databaseURL), clock)
	defer manager.Close()
	handler := NewHandler(manager, true, clock)
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverage(contract, []string{"/api/v1/public/documents/{key}", "/api/v1/public/documents"})
	response := summaryCall(handler, "/api/v1/public/documents/vn:vbpl:1")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"heading":"Article"`) || !strings.Contains(response.Body.String(), `"name":"Topic"`) {
		t.Fatalf("database response: %d %s", response.Code, response.Body.String())
	}
	coverage.Check(t, "GET", "/api/v1/public/documents/{key}", response)
	lease, err := manager.Resolve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var timeout, readOnly string
	publicPool := lease.entry.pool.(*postgresPool).conn
	if err = publicPool.QueryRow(ctx, `SELECT current_setting('statement_timeout'),current_setting('default_transaction_read_only')`).Scan(&timeout, &readOnly); err != nil || timeout != "2s" || readOnly != "on" || publicPool.Config().ConnectTimeout > 2*time.Second {
		t.Fatalf("public connection limits: %s %s %v", timeout, readOnly, err)
	}
	lease.Done()
	if _, err = pool.Exec(ctx, `INSERT INTO document_summary(document_key,indexed,projection) SELECT 'vn:vbpl:'||n,false,1 FROM generate_series(2,10000) n`); err != nil {
		t.Fatal(err)
	}
	items, err := p.List(ctx)
	if err != nil || len(items) != 10000 || items[0].Key != "vn:vbpl:1" || items[1].Key != "vn:vbpl:10" {
		t.Fatalf("list: %d %v", len(items), err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO document_summary(document_key,indexed,projection) VALUES('vn:vbpl:10001',false,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = p.List(ctx); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("unbounded list: %v", err)
	}
}
