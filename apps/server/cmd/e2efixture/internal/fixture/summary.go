//go:build integration

package fixture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func populateSummary(ctx context.Context, db *pgxpool.Pool) error {
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES (30,'vn','vn:fixture:public-summary','30/2099/TT-TEST','30/2099/TT-TEST','Văn bản tóm tắt E2E','test','Cơ quan giả lập','primary','vbpl',false,'2000-01-01','','','','','','in_force','source_status',true,false,'no_chunks',$1,'source_tree',true)`, []any{strings.Repeat("a", 64)}},
		{`INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (3001,30,'dieu-1','dieu','Điều 1','Điều khoản tóm tắt công khai E2E',1,1,'a1','E2E-BODY-MARKER-7Q4','dieu-1'), (3002,30,'dieu-2','dieu','Điều 2',$1,2,2,'a2','Nội dung giả lập.','dieu-2')`, []any{"E2E-REJECTED-HEADING-9K2" + strings.Repeat("x", 320-len("E2E-REJECTED-HEADING-9K2"))}},
		{`INSERT INTO taxonomy_node (id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (4,'vn',NULL,1,'99','Chủ đề giả lập','','phapdien','active','n4',2,1,1), (5,'vn',4,2,'99.1','Chủ đề tóm tắt E2E','','phapdien','active','n4.n5',1,1,1)`, nil},
		{`INSERT INTO taxonomy_assignment (provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (3001,5,5,'phapdien_seed',false,1,'v1')`, nil},
	}
	for _, q := range queries {
		if _, err := db.Exec(ctx, q.sql, q.args...); err != nil {
			return fmt.Errorf("insert summary fixture: %w", err)
		}
	}
	return nil
}
