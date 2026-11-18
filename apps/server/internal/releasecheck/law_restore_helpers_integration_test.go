//go:build integration

package releasecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func lawUpdater(t *testing.T, module, name string, args ...any) (string, string) {
	t.Helper()
	if args == nil {
		args = []any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	script := `import json,sys
sys.path.insert(0,sys.argv[1])
from updater import deploy_corpus_sql, deploy_corpus_verify, deploy_release_catalog, deploy_release_target
module={'sql':deploy_corpus_sql,'verify':deploy_corpus_verify,'catalog':deploy_release_catalog,'target':deploy_release_target}[sys.argv[2]]
value=getattr(module,sys.argv[3])
try:
    print(json.dumps({'value':value(*json.load(sys.stdin)) if callable(value) else value}))
except ValueError as error:
    print(json.dumps({'refused':str(error)}))
`
	cmd := exec.CommandContext(t.Context(), "python3", "-I", "-B", "-c", script, filepath.Join(repositoryRoot(t), "deploy/host"), module, name)
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run updater %s.%s", module, name)
	}
	var result struct {
		Value   *string `json:"value"`
		Refused *string `json:"refused"`
	}
	if err = json.Unmarshal(output, &result); err != nil || (result.Value == nil) == (result.Refused == nil) {
		t.Fatalf("decode updater %s.%s", module, name)
	}
	if result.Refused != nil {
		return "", *result.Refused
	}
	return *result.Value, ""
}

func lawSQL(t *testing.T, module, name string, args ...any) string {
	t.Helper()
	value, refused := lawUpdater(t, module, name, args...)
	if refused != "" {
		t.Fatalf("updater %s.%s refused: %s", module, name, refused)
	}
	return value
}

func lawClient(psql, program string) string {
	if filepath.IsAbs(psql) {
		return filepath.Join(filepath.Dir(psql), program)
	}
	return program
}

func lawRestore(t *testing.T, config *pgx.ConnConfig, restore, options string, dump []byte, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), restore, append([]string{"-h", config.Host, "-p", strconv.Itoa(int(config.Port)), "-U", config.User}, args...)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+config.Password, "PGOPTIONS="+options, "PGCONNECT_TIMEOUT=5")
	cmd.Stdin = bytes.NewReader(dump)
	if _, err := cmd.Output(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("pg_restore failed: %v: %.400s", err, bytes.TrimSpace(exitErr.Stderr))
		}
		t.Fatalf("pg_restore failed: %v", err)
	}
}

func seedLawCorpus(ctx context.Context, t *testing.T, db *pgx.Conn, rel string) {
	t.Helper()
	if err := corpusdb.Apply(ctx, db, 1024); err != nil {
		t.Fatal(err)
	}
	hash := strings.Repeat("a", 64)
	if _, err := db.Exec(ctx, `INSERT INTO document (id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,text_source,text_complete) VALUES (1,'vn','vn:synthetic','1','1','Quyền và nghĩa vụ','law','synthetic','primary','vbpl',false,'','','','','','in_force','source_status',true,false,'indexed',$1,'source_tree',true)`, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO provision (id,document_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES (1,1,'dieu-1','dieu','Điều 1','Quyền và nghĩa vụ',1,1,'law.article','Quyền và nghĩa vụ','dieu-1')`); err != nil {
		t.Fatal(err)
	}
	query, document := embedding.QueryRecipe1(), embedding.DocumentRecipe1()
	vector := make([]float32, query.Dimensions)
	vector[0] = 1
	marshal := func(v any) []byte {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if _, err := db.Exec(ctx, `INSERT INTO chunk (id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding,sparse) VALUES (1,1,1,0,'','Quyền và nghĩa vụ','[]',$1,4,$2::vector,'{1:1}/1048576'::sparsevec)`, hash, string(marshal(vector))); err != nil {
		t.Fatal(err)
	}
	dense := retrieval.DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Query: query, QueryHash: query.Hash(), Document: document, DocumentHash: document.Hash()}
	recipe := retrieval.Recipe3()
	threshold := 0.1
	recipe.Abstention.Threshold, recipe.Abstention.CalibrationSHA256 = &threshold, hash
	stats := lexical.StatsSummary{Version: 1, N: 1, TotalTokens: 4, AvgDL: 4, DistinctTerms: 1, OccupiedDimensions: 1, VocabularySHA256: hash, DFSHA256: hash}
	if _, err := db.Exec(ctx, `INSERT INTO release VALUES(true,'vn',$1,$2,'v1',$3,'{}',$4,$5,$6,$7,$7,$7,'{}',now())`, rel, corpusdb.SchemaVersion, marshal(dense), marshal(lexical.Recipe1()), marshal(recipe), marshal(stats), hash); err != nil {
		t.Fatal(err)
	}
}

func lawTableDigests(ctx context.Context, t *testing.T, conn *pgx.Conn) map[string]string {
	t.Helper()
	rows, err := conn.Query(ctx, `SELECT tablename::text FROM pg_catalog.pg_tables WHERE schemaname='public' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	digests := make(map[string]string, len(names))
	for _, name := range names {
		var digest string
		query := `SELECT count(*)::text||':'||coalesce(md5(string_agg(x::text, E'\n' ORDER BY x::text)),'') FROM public.` + pgx.Identifier{name}.Sanitize() + ` x`
		if err = conn.QueryRow(ctx, query).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		digests[name] = digest
	}
	return digests
}

func provisionerReset(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(t), "deploy/host/provision-activator.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "ALTER ROLE activator RESET ALL;")
	if start < 0 {
		t.Fatal("provisioner has no activator reset")
	}
	end := strings.Index(text[start:], `\gexec`)
	if end < 0 {
		t.Fatal("provisioner reset has no database step")
	}
	return text[start:start+end] + "\\gexec\n"
}
