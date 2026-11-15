//go:build integration

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCorrectionsCheck_ReleaseText(t *testing.T) {
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
	_, err = db.Exec(ctx, `CREATE TABLE document (id bigint PRIMARY KEY, key text, number text, eff_from date, eff_to date); CREATE TABLE provision (id bigint, document_id bigint, parent_id bigint, label text, anchor text, text text, preorder int); INSERT INTO document VALUES (1,'a','A',NULL,NULL),(2,'b','B',NULL,NULL); INSERT INTO provision VALUES (1,2,NULL,'Điều 1','dieu-1','exact text',1)`)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CORPUS_BUILDER_DATABASE_URL", base.String())
	dir := t.TempDir()
	if err = os.Mkdir(filepath.Join(dir, "sources"), 0700); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join("sources", "vn-corrections.yaml")
	data := "- {key: a, number: A, hold: true, reason: why, reviewed: '2099-01-01', evidence: {key: b, anchor: dieu-1, quote: 'exact  text'}}\n- {key: missing, number: M, hold: true, reason: why, reviewed: '2099-01-01', evidence: {url: 'https://example.org', quote: text}}\n"
	if err = os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	rel := strings.TrimPrefix(key, "vn/")
	var out, errout bytes.Buffer
	if code := Run(ctx, []string{"corrections", "check", "vn", rel}, &out, &errout); code != 1 || !strings.Contains(errout.String(), "evidence not found") || !strings.Contains(errout.String(), "document absent") {
		t.Fatalf("code=%d stderr=%s", code, errout.String())
	}
	data = strings.Split(data, "\n")[0] + "\n"
	data = strings.Replace(data, "exact  text", "exact text", 1)
	if err = os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := Run(ctx, []string{"corrections", "check", "vn", rel}, &out, &errout); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errout.String())
	}
	data = "- {key: a, number: A, set: {eff_from: '2025-01-01', eff_to: '2024-01-01'}, reason: why, reviewed: '2099-01-01', evidence: {key: b, anchor: dieu-1, quote: 'exact text'}}\n"
	if err = os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errout.Reset()
	if code := Run(ctx, []string{"corrections", "check", "vn", rel}, &out, &errout); code != 1 || !strings.Contains(errout.String(), "invalid interval") {
		t.Fatalf("code=%d stderr=%s", code, errout.String())
	}
	var text string
	if err = db.QueryRow(ctx, `SELECT text FROM provision WHERE document_id=2`).Scan(&text); err != nil || text != "exact text" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}
