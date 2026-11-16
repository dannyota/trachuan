//go:build integration

package classify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoadSelectedDocuments_AllSeededPrimaryAndIneligibleKeys(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testpg.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	_, err = db.Exec(ctx, `CREATE TABLE document(id bigint,key text,title text,class text,fetch_sha256 text);
CREATE TABLE provision(id bigint,document_id bigint,kind text,path text,anchor text,heading text,text text,label text,preorder bigint);
CREATE TABLE taxonomy_node(id bigint,code text,name text,parent_id bigint,level integer,status text,origin text,path text);
CREATE TABLE taxonomy_assignment(provision_id bigint,method text);
CREATE TABLE artifact(id bigint,stage text,sha256 text,path text);
INSERT INTO taxonomy_node VALUES(1,'1','Topic',NULL,1,'active','phapdien','n1'),(2,'1.1','Family',1,2,'active','phapdien','n1.n1');
INSERT INTO provision VALUES(1,1,'dieu','n1','dieu-1','Scope','Synthetic','Article 1',1),(2,2,'dieu','n1','dieu-1','Scope','Synthetic','Article 1',1),(3,3,'dieu','n1','dieu-1','Scope','Synthetic','Article 1',1);
INSERT INTO taxonomy_assignment VALUES(1,'phapdien_seed'),(2,'phapdien_seed'),(3,'phapdien_seed');`)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for id := 1; id <= 3; id++ {
		name := fmt.Sprint(id)
		dir := filepath.Join(root, name)
		if err = os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		body := fmt.Appendf(nil, `{"data":{"id":"%d","documentFields":[{"name":"Synthetic field"}]}}`, id)
		manifest := []byte(digest(body) + "  detail.json\n")
		if err = os.WriteFile(filepath.Join(dir, "detail.json"), body, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "MANIFEST.sha256"), manifest, 0600); err != nil {
			t.Fatal(err)
		}
		class := "primary"
		if id == 3 {
			class = "secondary"
		}
		if _, err = db.Exec(ctx, `INSERT INTO document VALUES($1,$2,'Synthetic title',$3,$4)`, id, "vn:vbpl:"+name, class, digest(manifest)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `INSERT INTO artifact VALUES($1,'fetch',$2,$3)`, id, digest(manifest), name); err != nil {
			t.Fatal(err)
		}
	}
	labels := taxonomy.Labels{Country: "vn", Release: testpg.Release(t)}
	for _, key := range []string{"vn:vbpl:2", "vn:vbpl:1"} {
		labels.Articles = append(labels.Articles, taxonomy.LabelArticle{Key: key, Anchor: "dieu-1", OwnTextSHA256: taxonomy.OwnTextSHA256("Synthetic"), Codes: []string{"1.1"}})
	}
	docs, err := LoadSelectedDocuments(ctx, db, db, artifact.Store{Root: root}, labels)
	if err != nil || len(docs) != 2 || docs[0].Key != "vn:vbpl:1" || docs[1].Key != "vn:vbpl:2" || len(docs[0].Fields) != 1 || len(docs[0].Contents) != 1 {
		t.Fatalf("documents %+v error %v", docs, err)
	}
	if _, err = LoadDocuments(ctx, db, db, artifact.Store{Root: root}, 1, 0); err == nil {
		t.Fatal("random draw included an all-seeded document")
	}
	if _, err = db.Exec(ctx, `INSERT INTO provision SELECT i,1,'dieu','n'||i,'dieu-'||(i-2),'Scope','Synthetic','Article',i FROM generate_series(4,22) i;
INSERT INTO taxonomy_assignment SELECT i,'phapdien_seed' FROM generate_series(4,22) i;`); err != nil {
		t.Fatal(err)
	}
	labels.Articles = labels.Articles[:1]
	for i := 2; i <= 20; i++ {
		labels.Articles = append(labels.Articles, taxonomy.LabelArticle{Key: "vn:vbpl:1", Anchor: fmt.Sprintf("dieu-%d", i), OwnTextSHA256: taxonomy.OwnTextSHA256("Synthetic"), Codes: []string{"1.1"}})
	}
	for _, key := range []string{"vn:vbpl:3", "vn:vbpl:absent"} {
		labels.Articles[0].Key = key
		if _, err = LoadSelectedDocuments(ctx, db, db, artifact.Store{Root: root}, labels); err == nil || !strings.Contains(err.Error(), "selected document "+key+" is absent or not primary") {
			t.Fatalf("ineligible key %s error %v", key, err)
		}
	}
}
