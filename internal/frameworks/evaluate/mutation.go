package evaluate

import (
	"context"
	"fmt"
	"strings"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/jackc/pgx/v5"
)

func versionCode(handle string) string {
	parts := strings.SplitN(handle, ":", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[1]
}
func mutationSuffix(index int) string { return fmt.Sprintf("_eval_mut_%d", index) }

func mutateClass(ctx context.Context, conn *pgx.Conn, caller serve.Caller, class string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return hide("begin invariant mutation", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, `SELECT id,framework_code,access FROM framework_version ORDER BY id`)
	if err != nil {
		return hide("read mutation versions", err)
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		var code, access string
		if rows.Scan(&id, &code, &access) != nil {
			rows.Close()
			return fmt.Errorf("read mutation version")
		}
		if !caller.Readable(code, access) {
			ids = append(ids, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return hide("read mutation versions", err)
	}
	queries := []string{
		`UPDATE control_text t SET body='Invariant replacement body',title_original='Invariant replacement title',notes=ARRAY['Invariant replacement note'],tables=ARRAY['Invariant replacement table'] FROM control c WHERE c.id=t.control_id AND c.version_id=ANY($1::bigint[])`,
		`UPDATE body_chunk b SET content='invariantreplacement',token_count=token_count+7,embedding=(SELECT array_agg(-entry ORDER BY ord)::vector FROM unnest(b.embedding::real[]) WITH ORDINALITY AS t(entry,ord)) WHERE version_id=ANY($1::bigint[])`,
		`DELETE FROM body_posting WHERE version_id=ANY($1::bigint[])`,
		`INSERT INTO body_posting(version_id,chunk_id,token,tf) SELECT version_id,id,'invariantreplacement',token_count FROM body_chunk WHERE version_id=ANY($1::bigint[])`,
		`UPDATE mapping_evidence SET evidence='Invariant replacement evidence' WHERE licensed_version_ids && $1::bigint[]`,
		`INSERT INTO mapping_evidence(mapping_id,evidence,licensed_version_ids) SELECT chosen.id,'Invariant added evidence',(SELECT array_agg(e.id ORDER BY e.id) FROM framework_version e WHERE e.access='licensed' AND e.id=ANY(ARRAY[chosen.from_version_id,chosen.to_version_id])) FROM framework_version v CROSS JOIN LATERAL (SELECT m.id,m.from_version_id,m.to_version_id FROM control_mapping m WHERE (m.from_version_id=v.id OR m.to_version_id=v.id) AND NOT EXISTS(SELECT 1 FROM mapping_evidence e WHERE e.mapping_id=m.id) ORDER BY m.id LIMIT 1) chosen WHERE v.id=ANY($1::bigint[]) AND NOT EXISTS(SELECT 1 FROM mapping_evidence e WHERE v.id=ANY(e.licensed_version_ids)) ON CONFLICT DO NOTHING`,
		`UPDATE quality_gap SET detail=detail||':1' WHERE class='text' AND version_id=ANY($1::bigint[])`,
		`INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) SELECT 'text_incomplete','text',v.id,'fw:'||v.framework_code||':'||v.version,'1',false FROM framework_version v WHERE v.id=ANY($1::bigint[]) AND NOT EXISTS(SELECT 1 FROM quality_gap g WHERE g.version_id=v.id AND g.class='text')`,
		`UPDATE source_file SET text_source=CASE WHEN text_source='ocr' THEN 'structured' ELSE 'ocr' END,text_complete=NOT text_complete WHERE version_id=ANY($1::bigint[])`,
	}
	classes := []string{"control_text", "body_index", "body_index", "body_index", "mapping_evidence", "mapping_evidence", "text_gaps", "text_gaps", "source_files"}
	for i, query := range queries {
		if classes[i] != class {
			continue
		}
		if _, err = tx.Exec(ctx, query, ids); err != nil {
			return &mutationFailure{class: classes[i], cause: err}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return hide("commit invariant mutation", err)
	}
	return nil
}

type mutationFailure struct {
	class string
	cause error
}

func (e *mutationFailure) Error() string { return "mutate " + e.class + " failed" }
func (e *mutationFailure) Unwrap() error { return e.cause }
