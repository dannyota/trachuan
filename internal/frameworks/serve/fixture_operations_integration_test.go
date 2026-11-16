//go:build integration

package serve

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func extendOperationFixture(ctx context.Context, conn *pgx.Conn, changed bool) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	term, repeats, detail := "quartz", 1500, "1"
	if changed {
		term, repeats, detail = "nebula", 2100, "20"
	}
	_, err = tx.Exec(ctx, `UPDATE control_text SET notes=$1,tables=$2 WHERE control_id=(
 SELECT c.id FROM control c JOIN framework_version v ON v.id=c.version_id
 WHERE v.framework_code='fw-alpha' AND v.version='2' AND c.citation='A-1')`,
		[]string{strings.Repeat("Invented "+term+" note. ", repeats)}, []string{"Invented " + term + " table row."})
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE source_file SET text_complete=$1 WHERE version_id=2`, !changed); err != nil {
		return err
	}
	for i := 1; i <= 205; i++ {
		source := fmt.Sprintf("invented-%03d-", i) + strings.Repeat("source", 20)
		_, err = tx.Exec(ctx, `INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved)
 VALUES($1,'fw-alpha','2','A-1','fw-beta','1','A-1','related',$2,2,3,true)`, i, source)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved) VALUES
 (206,'fw-beta','1','A-1','fw-alpha','2','A-1','equivalent','invented-public',3,2,true),
 (207,'fw-gamma','1','A-1','fw-delta','1','X-1','related','invented-unresolved',4,NULL,false)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO mapping_evidence(mapping_id,evidence,licensed_version_ids) VALUES
 (1,$1,ARRAY[2]::bigint[]),(206,'Invented public citation alignment.','{}'::bigint[])`, strings.Repeat("Invented "+term+" evidence. ", repeats))
	if err != nil {
		return err
	}
	if changed {
		_, err = tx.Exec(ctx, `INSERT INTO mapping_evidence(mapping_id,evidence,licensed_version_ids) VALUES(2,'Invented concealed evidence appears.',ARRAY[2]::bigint[])`)
		if err != nil {
			return err
		}
	}
	for i := 0; i < 205; i++ {
		_, err = tx.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks)
 VALUES('mapping_dropped','public',2,'A-1',$1,false)`, strconv.Itoa(i))
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks)
 VALUES('text_incomplete','text',2,'A-1',$1,false)`, detail)
	if err != nil {
		return err
	}
	if changed {
		_, err = tx.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks)
 VALUES('chunk_omission','text',2,'A-1','1',false)`)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
