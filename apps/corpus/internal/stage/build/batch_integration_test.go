//go:build integration

package build

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func batchCorpus(t *testing.T) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	c, err := pgx.Connect(ctx, testpg.NewDatabase(t, "vector", "ltree"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })
	if err = corpusdb.ApplyTables(ctx, c, 1024); err != nil {
		t.Fatal(err)
	}
	return c
}

func batchLoad(t *testing.T, f *loadFixture, c *pgx.Conn, limit int, options ...LoadOptions) error {
	t.Helper()
	_, err := loadWithBatchLimit(context.Background(), c, f.inventory(t), f.artifacts, f.recipes, map[string]source.StatusCode{"CCHL": {Class: "not_yet"}}, nil, f.asOf(), limit, options...)
	return err
}

func TestBatchLoad_EquivalentTables(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", false)
	f.indexed(t, "vn:vbpl:2", true)
	body := []byte("source fixture")
	hash := fmt.Sprintf("%x", sha256.Sum256(body))
	f.addArtifact(t, "fetch", "vn:vbpl:3", map[string]any{
		"detail.json":   map[string]any{"data": map[string]any{"id": "3"}},
		"files.json":    []any{map[string]any{"name": "law.pdf", "sha256": hash, "size": len(body)}},
		"files/" + hash: body,
	})
	addValidityFixture(t, f, "4", "1/2024/NĐ-CP", "CCHL", "2024-01-01", "2027-01-01", "Nghị định này có hiệu lực từ ngày 1/7/2024.")
	for _, item := range f.inventory(t).Items {
		if item.Stage == "fetch" && item.ItemKey == "vn:vbpl:1" {
			f.addArtifact(t, "relate", item.ItemKey, map[string]any{"relations.json": []relationRow{{Ordinal: 0, Kind: "cites", TargetID: "2"}}}, f.relateInput(t, item.ItemKey, *item.ArtifactSHA256))
			break
		}
	}
	input := taxonomyInputFixture()
	input.seeds = []taxonomySeed{{"1", "1", "1.1.c1"}, {"2", "1", "1.1.c1"}}
	input.overrides = []taxonomy.Override{{DocumentKey: "vn:vbpl:1", Anchor: "dieu-1", Code: "1.1.x1", Exclude: true}}
	opts := LoadOptions{Taxonomy: input}
	other := batchCorpus(t)
	if err := batchLoad(t, f, f.corpus, 1, opts); err != nil {
		t.Fatal(err)
	}
	if err := batchLoad(t, f, other, 2000, opts); err != nil {
		t.Fatal(err)
	}
	for _, entry := range [][2]string{{"document", "id"}, {"source_file", "id"}, {"provision", "id"}, {"chunk", "id"}, {"lexical_df", "token"}, {"relation", "id"}, {"coverage_gap", "id"}, {"taxonomy_node", "id"}, {"taxonomy_assignment", "provision_id,node_id"}, {"validity_decision", "document_id,ordinal"}} {
		table := entry[0]
		query := "SELECT to_jsonb(t)::text FROM " + pgx.Identifier{table}.Sanitize() + " t ORDER BY " + entry[1]
		var got [2][]string
		for i, c := range []*pgx.Conn{f.corpus, other} {
			rows, err := c.Query(context.Background(), query)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var row string
				if err = rows.Scan(&row); err != nil {
					t.Fatal(err)
				}
				got[i] = append(got[i], row)
			}
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
		}
		if len(got[0]) == 0 {
			t.Errorf("fixture did not populate %s", table)
		}
		if !reflect.DeepEqual(got[0], got[1]) {
			t.Errorf("%s differs: %v versus %v", table, got[0], got[1])
		}
	}
}

func TestBatchLoad_FailureRollsBack(t *testing.T) {
	f := fixture(t)
	for _, key := range []string{"vn:vbpl:1", "vn:vbpl:2", "vn:vbpl:3"} {
		f.fetch(t, key)
	}
	other := batchCorpus(t)
	var errors [2]string
	for i, c := range []*pgx.Conn{f.corpus, other} {
		_, err := c.Exec(context.Background(), `CREATE FUNCTION reject_second() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id=2 THEN RAISE EXCEPTION 'second document rejected'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_second BEFORE INSERT ON document FOR EACH ROW EXECUTE FUNCTION reject_second()`)
		if err != nil {
			t.Fatal(err)
		}
		limit := []int{1, 2000}[i]
		err = batchLoad(t, f, c, limit)
		if err == nil {
			t.Fatal("expected load failure")
		}
		errors[i] = err.Error()
		var count int
		if err = c.QueryRow(context.Background(), `SELECT count(*) FROM document`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("documents after failure = %d: %v", count, err)
		}
	}
	if errors[0] != errors[1] || !strings.Contains(errors[0], "insert document: ") || !strings.Contains(errors[0], "second document rejected") {
		t.Fatalf("errors differ: %q and %q", errors[0], errors[1])
	}
}

func TestBatchLoad_ReadSeesQueuedChunks(t *testing.T) {
	f := fixture(t)
	f.indexed(t, "vn:vbpl:1", false)
	if err := batchLoad(t, f, f.corpus, 2000); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM chunk`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("chunks = %d: %v", n, err)
	}
}
