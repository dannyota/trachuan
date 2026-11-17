//go:build integration

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type frameworkLawTexts struct{ values []string }

func (i *frameworkLawTexts) Next(context.Context) (string, bool, error) {
	if len(i.values) == 0 {
		return "", false, nil
	}
	text := i.values[0]
	i.values = i.values[1:]
	return text, true, nil
}

func frameworkLawEngine(t *testing.T) *retrieval.Engine {
	t.Helper()
	ctx := t.Context()
	query, document := embedding.QueryRecipe1(), embedding.DocumentRecipe1()
	db, err := pgxpool.New(ctx, testpg.NewDatabase(t, "vector", "ltree"))
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, corpusdb.Apply(ctx, db, query.Dimensions))
	texts := []string{strings.Repeat("Văn bản ngân hàng giả lập. ", 2500), "Văn bản ngân hàng giả lập thứ hai."}
	stats, err := lexical.Train(ctx, &frameworkLawTexts{values: texts}, lexical.Recipe1())
	require.NoError(t, err)
	exec := func(statement string, args ...any) {
		t.Helper()
		_, execErr := db.Exec(ctx, statement, args...)
		require.NoError(t, execErr)
	}
	hash := strings.Repeat("a", 64)
	for term, df := range stats.DF {
		exec(`INSERT INTO lexical_df(token,dimension,df) VALUES($1,$2,$3)`, term, lexical.Dimension(term, lexical.Recipe1()), df)
	}
	for i, text := range texts {
		id := i + 1
		exec(`INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,eff_from,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete)
 VALUES($1,'vn',$2,$3,$3,'Văn bản giả lập','law','invented','primary','vbpl',false,'2099-01-01','','','','','','in_force','source_status',true,false,'indexed',$4,'source_tree',true)`, id, fmt.Sprintf("vn:invented:%d", id), fmt.Sprintf("%d/2099/TT-TEST", id), hash)
		exec(`INSERT INTO provision(id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES($1,$1,'dieu-1','dieu','Điều 1','',1,1,'p',$2,'dieu-1')`, id, text)
		sparse, sparseErr := lexical.Document(text, stats, lexical.Recipe1())
		require.NoError(t, sparseErr)
		var terms []string
		for _, term := range sparse {
			terms = append(terms, strconv.FormatInt(int64(term.Index), 10)+":"+strconv.FormatFloat(float64(term.Value), 'g', -1, 32))
		}
		sparseText := "{" + strings.Join(terms, ",") + "}/" + strconv.Itoa(lexical.Recipe1().Dimensions)
		vector := "[1" + strings.Repeat(",0", query.Dimensions-1) + "]"
		exec(`INSERT INTO chunk(id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES($1,$1,$1,0,'',$2,'[]',$3,1,$4::vector,$5::sparsevec)`, id, text, hash, vector, sparseText)
	}
	recipe := retrieval.Recipe2()
	threshold := 0.1
	recipe.Abstention.Threshold, recipe.Abstention.CalibrationSHA256 = &threshold, hash
	dense := retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Document: document, DocumentHash: document.Hash(), Query: query, QueryHash: query.Hash()}
	marshal := func(value any) []byte {
		t.Helper()
		raw, marshalErr := json.Marshal(value)
		require.NoError(t, marshalErr)
		return raw
	}
	exec(`INSERT INTO release VALUES(true,'vn','20990101',$1,'v1',$2,'{}',$3,$4,$5,$6,$6,$6,'{}','2099-01-01')`, corpusdb.SchemaVersion, marshal(dense), marshal(lexical.Recipe1()), marshal(recipe), marshal(stats.Summary(lexical.Recipe1(), nil)), hash)
	engine, err := retrieval.Open(ctx, db, frameworkFixtureQuery{}, retrieval.Options{Now: func() time.Time { return time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC) }, Profile: "evaluation"})
	require.NoError(t, err)
	t.Cleanup(engine.Close)
	return engine
}
