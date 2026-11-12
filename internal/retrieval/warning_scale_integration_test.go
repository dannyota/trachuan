//go:build integration

package retrieval

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type warningRowsTrace struct {
	queries int
	rows    int64
}
type warningRowsKey struct{}

func (tr *warningRowsTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, warningRowsKey{}, strings.Contains(data.SQL, "relation "))
}
func (tr *warningRowsTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if matched, _ := ctx.Value(warningRowsKey{}).(bool); matched {
		tr.queries++
		tr.rows += data.CommandTag.RowsAffected()
	}
}

func TestReadWarnings_UnrelatedRelationsStayBounded(t *testing.T) {
	db, _, _ := fixture(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) SELECT 1000+n,1,11,'scale-'||n,'dieu','article','',n+10,n+10,('d1.x'||n)::ltree,'body','scale-'||n FROM generate_series(1,1000) n`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) SELECT n,2,1,'vn:2','vn:1','n','n','cites','','',repeat('e',1000),'vbpl_diagram',repeat('b',64),n FROM generate_series(1,1000) n`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_node_id,evidence,method,artifact_sha256,ordinal) SELECT 1000+n,2,1,1001,'vn:2','vn:1','n','n','repeals','','','scale-1','evidence','text_rule/1',repeat('b',64),1000+n FROM generate_series(1,15) n`)
	if err != nil {
		t.Fatal(err)
	}
	trace := &warningRowsTrace{}
	cfg := db.Config()
	cfg.ConnConfig.Tracer = trace
	traced, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer traced.Close()
	tx, err := traced.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	nodes, err := readNodes(ctx, tx, 1, "vn:1")
	if err != nil {
		t.Fatal(err)
	}
	units := make([]Unit, 1000)
	for i := range units {
		units[i] = Unit{Kind: "outline", Anchor: fmt.Sprintf("scale-%d", i+1)}
	}
	if err = attachReadWarnings(ctx, tx, nodes, units); err != nil {
		t.Fatal(err)
	}
	if trace.queries == 0 || trace.rows != 10 {
		t.Fatalf("relation query returned %d rows across %d statements", trace.rows, trace.queries)
	}
	if len(units[0].Warnings) != 10 || units[0].WarningsOmitted != 5 {
		t.Fatalf("capped warnings: %+v", units[0])
	}
	for _, unit := range units[1:] {
		if len(unit.Warnings) != 0 || unit.WarningsOmitted != 0 {
			t.Fatalf("unexpected warnings: %+v", unit)
		}
	}
}
