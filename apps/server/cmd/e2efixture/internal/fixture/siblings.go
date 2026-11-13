//go:build integration

package fixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgxpool"
)

const siblingLead = "Điều kiện giả lập cho các khoản."

func siblingTexts() []string {
	return []string{siblingLead + "\n" + strings.Repeat("Khoản đầu chứa nội dung giả lập dài. ", 40), siblingLead + "\n" + "Khoản hai có dấu hiệu hợp nhất màu ngọc."}
}

func populateSiblings(ctx context.Context, db *pgxpool.Pool, stats lexical.Stats, lex lexical.Recipe) ([]int, error) {
	hash := strings.Repeat("a", 64)
	_, err := db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES (29,'vn','vn:fixture:siblings','29/2099/TT-TEST','29/2099/TT-TEST','Văn bản hợp nhất giả lập','test','Cơ quan giả lập','primary','vbpl',false,'2000-01-01','','','','','','in_force','source_status',true,false,'indexed',$1,'source_tree',true)`, hash)
	if err != nil {
		return nil, fmt.Errorf("insert sibling document: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (2900,29,'dieu-1','dieu','Điều 1','',1,1,'a1',$1,'dieu-1')`, siblingLead)
	if err != nil {
		return nil, fmt.Errorf("insert sibling article: %w", err)
	}
	var nnz []int
	for i, content := range append(siblingTexts(), "Khoản ba chỉ có trong bài đọc đầy đủ.") {
		id, ordinal := 2901+i, i+1
		body := content
		if i < 2 {
			body = strings.TrimPrefix(content, siblingLead+"\n")
		}
		node := fmt.Sprintf("clause-%d", ordinal)
		_, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES ($1,29,2900,$2,'khoan',$3,'',$4,$5,$6::ltree,$7,$2)`, id, node, fmt.Sprintf("Khoản %d", ordinal), ordinal, ordinal+1, fmt.Sprintf("a1.c%d", ordinal), body)
		if err != nil {
			return nil, fmt.Errorf("insert sibling clause: %w", err)
		}
		if i == 2 {
			continue
		}
		sparse, lexErr := lexical.Document(content+" "+contextPrefix, stats, lex)
		if lexErr != nil {
			return nil, fmt.Errorf("encode sibling vector: %w", lexErr)
		}
		nnz = append(nnz, len(sparse))
		input := contextPrefix + "\n" + content
		sum := sha256.Sum256([]byte(input))
		leadEnd := len([]rune(siblingLead))
		end := len([]rune(content))
		spans, _ := json.Marshal([]map[string]any{{"node_id": "dieu-1", "start": 0, "end": leadEnd, "content_start": 0, "content_end": leadEnd, "role": "lead"}, {"node_id": node, "start": 0, "end": len([]rune(body)), "content_start": leadEnd + 1, "content_end": end, "role": "body"}})
		_, err = db.Exec(ctx, `INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES ($1,29,$2,$3,$4,$5,$6::jsonb,$7,$8,$9::vector,$10::sparsevec)`, 37+i, id, ordinal, contextPrefix, content, spans, hex.EncodeToString(sum[:]), len([]rune(input))+2, denseLiteral(embedding.FakeVector(input)), sparseLiteral(sparse, lex.Dimensions))
		if err != nil {
			return nil, fmt.Errorf("insert sibling chunk: %w", err)
		}
	}
	return nnz, nil
}
