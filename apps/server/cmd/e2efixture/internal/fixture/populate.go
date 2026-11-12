//go:build integration

package fixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgxpool"
)

const contextPrefix = "Văn bản thử nghiệm"

type textIterator struct {
	texts []string
	index int
}

func (it *textIterator) Next(context.Context) (string, bool, error) {
	if it.index == len(it.texts) {
		return "", false, nil
	}
	text := it.texts[it.index]
	it.index++
	return text, true, nil
}

func populate(ctx context.Context, db *pgxpool.Pool, rel string) error {
	hash := strings.Repeat("a", 64)
	texts := []string{"Mẫu giả lập vườn thử nghiệm xanh", "Mẫu giả lập về chiếc thuyền giấy màu tím.", "Mẫu giả lập về ngọn đèn bằng gỗ.", "Mẫu giả lập về bản đồ các vì sao.", "Mẫu giả lập về chiếc hộp màu cam.", "Mẫu giả lập về cây bút làm bằng mây.", "Mẫu giả lập về bức tranh con mèo.", "Mẫu giả lập về con đường lát kẹo.", "Mẫu giả lập về hồ nước bằng giấy.", "Mẫu giả lập về đồng hồ mặt trăng.", "Mẫu giả lập về đàn chim bằng vải.", "Mẫu giả lập về quả bóng bằng lá."}
	for i := 1; i <= 24; i++ {
		texts = append(texts, fmt.Sprintf("Mẫu phân loại giả lập số %d.", i))
	}
	lex := lexical.Recipe1()
	lexicalTexts := make([]string, len(texts))
	for i, text := range texts {
		lexicalTexts[i] = text + " " + contextPrefix
	}
	stats, err := lexical.Train(ctx, &textIterator{texts: lexicalTexts}, lex)
	if err != nil {
		return fmt.Errorf("train fixture statistics: %w", err)
	}
	for token, df := range stats.DF {
		if _, execErr := db.Exec(ctx, `INSERT INTO lexical_df (token,dimension,df) VALUES ($1,$2,$3)`, token, lexical.Dimension(token, lex), df); execErr != nil {
			return fmt.Errorf("insert fixture term: %w", execErr)
		}
	}
	nnz := make([]int, 0, len(texts))
	for doc := 1; doc <= 3; doc++ {
		number := fmt.Sprintf("%d/2099/TT-TEST", doc)
		key := fmt.Sprintf("vn:fixture:%d", doc)
		_, err = db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES ($1,'vn',$2,$3,$3,'Văn bản giả lập thử nghiệm','test','Cơ quan giả lập','primary','vbpl',false,'2000-01-01','','','','','','in_force','source_status',true,false,'indexed',$4,'source_tree',true)`, doc, key, number, hash)
		if err != nil {
			return fmt.Errorf("insert fixture document: %w", err)
		}
		for chapter := 1; chapter <= 2; chapter++ {
			parent := doc*100 + chapter*10
			chapterNode := fmt.Sprintf("chapter-%d", chapter)
			_, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES ($1,$2,$3,'chuong',$4,'Chương giả lập',$5,$6,$7::ltree,'Nội dung giả lập.',$3)`, parent, doc, chapterNode, fmt.Sprintf("Chương %d", chapter), chapter, (chapter-1)*3+1, fmt.Sprintf("c%d", chapter))
			if err != nil {
				return fmt.Errorf("insert fixture chapter: %w", err)
			}
			for article := 1; article <= 2; article++ {
				index := (doc-1)*4 + (chapter-1)*2 + article - 1
				id := parent + article
				node := fmt.Sprintf("article-%d", (chapter-1)*2+article)
				text := texts[index]
				_, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES ($1,$2,$3,$4,'dieu',$5,'Điều giả lập',$6,$7,$8::ltree,$9,$4)`, id, doc, parent, node, fmt.Sprintf("Điều %d", (chapter-1)*2+article), article, (chapter-1)*3+article+1, fmt.Sprintf("c%d.a%d", chapter, article), text)
				if err != nil {
					return fmt.Errorf("insert fixture article: %w", err)
				}
				sparse, lexErr := lexical.Document(lexicalTexts[index], stats, lex)
				if lexErr != nil {
					return fmt.Errorf("encode fixture lexical vector: %w", lexErr)
				}
				nnz = append(nnz, len(sparse))
				input := contextPrefix + "\n" + text
				inputSum := sha256.Sum256([]byte(input))
				_, err = db.Exec(ctx, `INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES ($1,$2,$3,$4,$10,$5,'[]',$6,$7,$8::vector,$9::sparsevec)`, index+1, doc, id, index%4, text, hex.EncodeToString(inputSum[:]), len([]rune(input))+2, denseLiteral(embedding.FakeVector(input)), sparseLiteral(sparse, lex.Dimensions), contextPrefix)
				if err != nil {
					return fmt.Errorf("insert fixture chunk: %w", err)
				}
			}
		}
	}
	for i := 1; i <= 24; i++ {
		doc, id, index := i+3, 1000+i, i+11
		key, number := fmt.Sprintf("vn:fixture:%d", doc), fmt.Sprintf("%d/2099/TT-TEST", doc)
		_, err = db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES ($1,'vn',$2,$3,$3,'Văn bản phân loại giả lập','test','Cơ quan giả lập','primary','vbpl',false,'2000-01-01','','','','','','in_force','source_status',true,false,'indexed',$4,'source_tree',true)`, doc, key, number, hash)
		if err != nil {
			return fmt.Errorf("insert taxonomy fixture document: %w", err)
		}
		_, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES ($1,$2,'article-1','dieu','Điều 1','Điều giả lập',1,1,'a1',$3,'article-1')`, id, doc, texts[index])
		if err != nil {
			return fmt.Errorf("insert taxonomy fixture article: %w", err)
		}
		sparse, lexErr := lexical.Document(lexicalTexts[index], stats, lex)
		if lexErr != nil {
			return fmt.Errorf("encode taxonomy fixture lexical vector: %w", lexErr)
		}
		nnz = append(nnz, len(sparse))
		input := contextPrefix + "\n" + texts[index]
		inputSum := sha256.Sum256([]byte(input))
		_, err = db.Exec(ctx, `INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES ($1,$2,$3,0,$4,$5,'[]',$6,$7,$8::vector,$9::sparsevec)`, index+1, doc, id, contextPrefix, texts[index], hex.EncodeToString(inputSum[:]), len([]rune(input))+2, denseLiteral(embedding.FakeVector(input)), sparseLiteral(sparse, lex.Dimensions))
		if err != nil {
			return fmt.Errorf("insert taxonomy fixture chunk: %w", err)
		}
	}
	_, err = db.Exec(ctx, `INSERT INTO taxonomy_node (id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES (1,'vn',NULL,1,'90','Chủ đề thử nghiệm 90','','phapdien','active','n1',1,1,1), (2,'vn',1,2,'90.1','Đề mục thử nghiệm 90.1','','phapdien','active','n1.n2',1,1,1), (3,'vn',2,3,'90.1.c1','Chương thử nghiệm 90.1.c1','','phapdien','active','n1.n2.n3',1,1,1)`)
	if err != nil {
		return fmt.Errorf("insert fixture taxonomy: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO taxonomy_assignment (provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (1001,3,2,'phapdien_seed',false,1,'v1')`)
	if err != nil {
		return fmt.Errorf("insert fixture assignment: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,evidence,method,artifact_sha256,ordinal) VALUES (1,1,2,'vn:fixture:1','vn:fixture:2','1/2099/TT-TEST','2/2099/TT-TEST','references','','','Mối liên hệ giả lập.','source',$1,0)`, hash)
	if err != nil {
		return fmt.Errorf("insert fixture relation: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES (28,'vn','vn:fixture:warnings','28/2099/TT-TEST','28/2099/TT-TEST','Văn bản cảnh báo giả lập','test','Cơ quan giả lập','relation_context','vbpl',false,'2000-01-01','','','','','','in_force','source_status',true,false,'no_chunks',$1,'source_tree',true)`, hash)
	if err != nil {
		return fmt.Errorf("insert warning fixture document: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (2801,28,'article-1','dieu','Điều 1','',1,1,'a1','Nội dung giả lập.','article-1')`)
	if err != nil {
		return fmt.Errorf("insert warning fixture provision: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO relation (id,source_document_id,target_document_id,source_provision_id,target_provision_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES
 (2,28,1,2801,111,'vn:fixture:warnings','vn:fixture:1','28/2099/TT-TEST','1/2099/TT-TEST','amends_supplements','','','article-1','exact evidence','text_rule/1',$1,1),
 (3,28,1,2801,111,'vn:fixture:warnings','vn:fixture:1','28/2099/TT-TEST','1/2099/TT-TEST','repeals','','','article-1/khoan-9','inexact evidence','text_rule/1',$1,2),
 (4,28,1,2801,111,'vn:fixture:warnings','vn:fixture:1','28/2099/TT-TEST','1/2099/TT-TEST','replaces','','','vbpl-node','reference evidence','vbpl_references,vbpl_diagram',$1,3),
 (5,28,1,2801,111,'vn:fixture:warnings','vn:fixture:1','28/2099/TT-TEST','1/2099/TT-TEST','suspends','','','article-1','diagram evidence','vbpl_diagram',$1,4)`, hash)
	if err != nil {
		return fmt.Errorf("insert fixture warning relations: %w", err)
	}
	q := embedding.TestRecipe()
	document := q
	document.Runtime = "fake-document"
	dense := retrieval.DenseRecipe{Version: 1, Model: q.Model, Dimensions: q.Dimensions, Document: document, DocumentHash: document.Hash(), Query: q, QueryHash: q.Hash()}
	recipe := retrieval.Recipe1()
	threshold := 0.1
	recipe.Abstention.Threshold = &threshold
	recipe.Abstention.CalibrationSHA256 = hash
	marshal := func(v any) ([]byte, error) { return json.Marshal(v) }
	denseJSON, err := marshal(dense)
	if err != nil {
		return fmt.Errorf("marshal dense recipe: %w", err)
	}
	lexJSON, err := marshal(lex)
	if err != nil {
		return fmt.Errorf("marshal lexical recipe: %w", err)
	}
	recipeJSON, err := marshal(recipe)
	if err != nil {
		return fmt.Errorf("marshal retrieval recipe: %w", err)
	}
	statsJSON, err := marshal(stats.Summary(lex, nnz))
	if err != nil {
		return fmt.Errorf("marshal lexical statistics: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO release VALUES (true,'vn',$1,$2,'v1',$3::jsonb,'{}',$4::jsonb,$5::jsonb,$6::jsonb,$7,$7,$7,'{}',now())`, rel, corpusdb.SchemaVersion, denseJSON, lexJSON, recipeJSON, statsJSON, hash)
	if err != nil {
		return fmt.Errorf("insert fixture release: %w", err)
	}
	return nil
}

func denseLiteral(values []float32) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.FormatFloat(float64(v), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
func sparseLiteral(values lexical.Vector, dimensions int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.FormatInt(int64(v.Index), 10) + ":" + strconv.FormatFloat(float64(v.Value), 'g', -1, 32)
	}
	return "{" + strings.Join(parts, ",") + "}/" + strconv.Itoa(dimensions)
}
