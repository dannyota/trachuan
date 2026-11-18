//go:build integration

package cli

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/knowledge"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestKnowledgeCommands_ReadReleaseReadOnly(t *testing.T) {
	ctx := context.Background()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	name, key, err := testpg.CreateCorpus(ctx, admin, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer testpg.DropCorpusDatabase(t, admin, name, key)
	base.Path = "/" + name
	db, err := pgxpool.New(ctx, base.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(ctx, `CREATE TABLE document (id bigint PRIMARY KEY, key text, number text, number_norm text, text_source text, text_complete boolean);
CREATE TABLE provision (id bigint PRIMARY KEY, document_id bigint, parent_id bigint, node_id text, anchor text, heading text, text text, preorder int);
CREATE TABLE relation (kind text, method text, source_document_id bigint, target_document_id bigint, source_provision_id bigint, target_provision_id bigint, target_node_id text);
CREATE TABLE coverage_gap (stage text, kind text, item_key text);
CREATE TABLE release (singleton boolean, country text, rel text, schema_version integer);
INSERT INTO document VALUES (1,'vn:vbpl:1','1/2099','1/2099','source_tree',true),(2,'vn:vbpl:2','2/2099','2/2099','source_tree',true);
INSERT INTO provision VALUES (10,1,NULL,'t','dieu-5','Tiêu đề','Văn bản đích.',1),(20,2,NULL,'a','dieu-30','','Bãi bỏ Điều 5 của văn bản đích.',1)`)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORPUS_BUILDER_DATABASE_URL", base.String())
	knowledgeDir(t, "[]\n")
	rel := strings.TrimPrefix(key, "vn/")
	if code, _, errout := execKnowledge(t, "check", "vn", rel); code != 1 || !strings.Contains(errout, "vn/"+rel) {
		t.Fatalf("check without release row: code=%d err=%q", code, errout)
	}
	if code, _, errout := execKnowledge(t, "depends", "vn", rel, "vn:vbpl:1", "dieu-5"); code != 1 || !strings.Contains(errout, "vn/"+rel) {
		t.Fatalf("depends without release row: code=%d err=%q", code, errout)
	}
	if _, err = db.Exec(ctx, `INSERT INTO release VALUES (true,'vn',$1,$2)`, rel, corpusdb.SchemaVersion-1); err != nil {
		t.Fatal(err)
	}
	if code, _, errout := execKnowledge(t, "check", "vn", rel); code != 1 || !strings.Contains(errout, "vn/"+rel) {
		t.Fatalf("check at another schema: code=%d err=%q", code, errout)
	}
	if _, err = db.Exec(ctx, `UPDATE release SET schema_version=$1`, corpusdb.SchemaVersion); err != nil {
		t.Fatal(err)
	}
	if code, out, errout := execKnowledge(t, "check", "vn", rel); code != 0 || !strings.Contains(out, "applied 0, redundant 0, stale 0") {
		t.Fatalf("check with release row: code=%d out=%q err=%q", code, out, errout)
	}
	readOnly, err := openKnowledgeRelease(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	var setting string
	if err = readOnly.QueryRow(ctx, `SHOW default_transaction_read_only`).Scan(&setting); err != nil || setting != "on" {
		t.Fatalf("default_transaction_read_only = %q, %v", setting, err)
	}
	code, out, errout := execKnowledge(t, "depends", "vn", rel, "vn:vbpl:1", "dieu-5")
	if code != 0 || !strings.Contains(out, "text_sha256: "+knowledge.Hash("Văn bản đích.")) {
		t.Fatalf("depends code=%d out=%q err=%q", code, out, errout)
	}
	if code, _, errout = execKnowledge(t, "depends", "vn", rel, "vn:vbpl:1", "dieu-9"); code != 1 || !strings.Contains(errout, knowledge.ReasonAnchorAbsent) {
		t.Fatalf("absent code=%d err=%q", code, errout)
	}
	acting := dependsBlock(t, rel, "vn:vbpl:2", "dieu-30")
	target := dependsBlock(t, rel, "vn:vbpl:1", "dieu-5")
	entry := `- id: "effect-mau"
  kind: "provision_effect"
  target: { key: "vn:vbpl:1", anchor: "dieu-5" }
  claim: { acting: { key: "vn:vbpl:2", anchor: "dieu-30" }, relation_kind: "repeals" }
  evidence: { quote: "Bãi bỏ Điều 5 của văn bản đích." }
  depends:
    contract: 1
    acting:
` + acting + `    target:
` + target + `  reviewed: { date: "2099-11-18", by: "primary", method: "human" }
`
	knowledgeDir(t, entry)
	code, out, errout = execKnowledge(t, "check", "vn", rel)
	if code != 0 || !strings.Contains(out, "effect-mau\tprovision_effect\tvn:vbpl:1\tapplied") {
		t.Fatalf("check code=%d out=%q err=%q", code, out, errout)
	}
	if _, err = db.Exec(ctx, `UPDATE provision SET text='Văn bản đã đổi.' WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	code, out, _ = execKnowledge(t, "check", "vn", rel)
	if code != 0 || !strings.Contains(out, "stale\ttext") {
		t.Fatalf("stale code=%d out=%q", code, out)
	}
}

func dependsBlock(t *testing.T, rel, key, anchor string) string {
	t.Helper()
	code, out, errout := execKnowledge(t, "depends", "vn", rel, key, anchor)
	if code != 0 {
		t.Fatalf("depends %s: code=%d err=%q", key, code, errout)
	}
	return "      " + strings.ReplaceAll(strings.TrimSpace(out), "\n", "\n      ") + "\n"
}
