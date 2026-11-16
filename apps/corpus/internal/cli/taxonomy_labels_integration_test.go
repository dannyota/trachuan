//go:build integration

package cli

import (
	"context"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSeededArticles_Pool(t *testing.T) {
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
	_, err = db.Exec(ctx, `CREATE TABLE document (id bigint, key text, class text); CREATE TABLE provision (id bigint, document_id bigint, anchor text, text text, kind text); CREATE TABLE taxonomy_node (id bigint, code text); CREATE TABLE taxonomy_assignment (provision_id bigint, node_id bigint, method text);
INSERT INTO document VALUES (1,'a','primary'),(2,'b','secondary');
INSERT INTO provision VALUES (1,1,'dieu-1','abc','dieu'),(2,1,'khoan-1','child','khoan'),(3,2,'dieu-1','secondary','dieu'),(4,1,'dieu-2','unseeded','dieu');
INSERT INTO taxonomy_node VALUES (1,'2.1'),(2,'1.1');
INSERT INTO taxonomy_assignment VALUES (1,1,'phapdien_seed'),(1,2,'phapdien_seed'),(1,1,'phapdien_seed'),(2,1,'phapdien_seed'),(3,1,'phapdien_seed'),(4,1,'manual');`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := seededArticles(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	want := []taxonomy.LabelArticle{{Key: "a", Anchor: "dieu-1", OwnTextSHA256: "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", Codes: []string{"1.1", "2.1"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
