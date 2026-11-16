//go:build integration

package corpusdb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSummary_Constraints(t *testing.T) {
	db := schemaDB(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,validity_conflict,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES(1,'vn','vn:vbpl:1','','','','','','primary','vbpl','','','','','','unknown','source_status',false,false,false,'no_tree',repeat('a',64),'none',false)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO document_summary(document_key,class,projection,indexed,status_class,status_basis,is_current,validity_conflict,gap_kinds,payload) VALUES('vn:vbpl:1','primary',1,false,'unknown','source_status',false,false,'{}','{}')`)
	if err != nil {
		t.Fatal(err)
	}
	for name, query := range map[string]string{
		"class":             `UPDATE document_summary SET class='bad'`,
		"projection":        `UPDATE document_summary SET projection=2`,
		"secondary indexed": `UPDATE document_summary SET class='secondary',indexed=true`,
		"context indexed":   `UPDATE document_summary SET class='relation_context',indexed=true`,
		"status":            `UPDATE document_summary SET status_class='bad'`,
		"basis":             `UPDATE document_summary SET status_basis='bad'`,
		"current":           `UPDATE document_summary SET is_current=true`,
		"dates":             `UPDATE document_summary SET eff_from='2099-01-02',eff_to='2099-01-01'`,
		"gap type":          `UPDATE document_summary SET gap_kinds=ARRAY['no_tree']`,
		"gap order":         `UPDATE document_summary SET gap_kinds=ARRAY['consolidated_status_unresolved','consolidated_status_unordered']`,
		"gap duplicate":     `UPDATE document_summary SET gap_kinds=ARRAY['consolidated_status_unordered','consolidated_status_unordered']`,
		"gap null element":  `UPDATE document_summary SET gap_kinds=ARRAY[NULL]::text[]`,
		"payload array":     `UPDATE document_summary SET payload='[]'`,
		"payload null":      `UPDATE document_summary SET payload='null'`,
		"payload octets":    `UPDATE document_summary SET payload=jsonb_build_object('text',repeat('ố',22000))`,
		"missing document":  `UPDATE document_summary SET document_key='vn:vbpl:2'`,
	} {
		t.Run(name, func(t *testing.T) {
			_, e := db.Exec(ctx, query)
			var pgErr *pgconn.PgError
			if !errors.As(e, &pgErr) || !strings.HasPrefix(pgErr.Code, "23") {
				t.Fatalf("expected constraint error: %v", e)
			}
		})
	}
	for _, column := range []string{"document_key", "class", "projection", "indexed", "status_class", "status_basis", "is_current", "validity_conflict", "gap_kinds", "payload"} {
		t.Run("null "+column, func(t *testing.T) {
			if _, e := db.Exec(ctx, "UPDATE document_summary SET "+column+"=NULL"); e == nil {
				t.Fatal("accepted null")
			}
		})
	}
	for _, gaps := range []string{`ARRAY[]::text[]`, `ARRAY['consolidated_status_unordered']`, `ARRAY['consolidated_status_unresolved']`, `ARRAY['consolidated_status_unordered','consolidated_status_unresolved']`} {
		if _, err = db.Exec(ctx, `UPDATE document_summary SET gap_kinds=`+gaps); err != nil {
			t.Fatal(err)
		}
	}
}
