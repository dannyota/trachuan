package build

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type noneTx struct {
	pgx.Tx
	inserts int
}

func (tx *noneTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	tx.inserts++
	return pgconn.CommandTag{}, nil
}

func TestLoad_RejectsProvisionsForNoneSource(t *testing.T) {
	ctx := context.Background()
	var store artifact.Store
	recipe := strings.Repeat("a", 64)
	publish := func(stage, input string, files map[string]string) artifact.Result {
		t.Helper()
		art, err := store.Publish(ctx, stage, input, recipe, func(dir string) error {
			for name, body := range files {
				if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
					return e
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return art
	}
	for _, tc := range []struct{ name, entries string }{{"with locator", `[{"NodeID":"tx:1","StartOrdinal":1,"EndOrdinal":1}]`}, {"without locator", `[]`}} {
		t.Run(tc.name, func(t *testing.T) {
			store = artifact.Store{Root: t.TempDir()}
			f := publish("fetch", strings.Repeat("b", 64), map[string]string{"detail.json": `{"data":{"id":"1"}}`, "files.json": `[]`})
			input := fetch.ExtractInputHash(f.SHA256, "none", "no_text_source")
			e := publish("extract", input, map[string]string{"text.json": `{"text_source":"none","gap":"no_text_source","provisions":` + tc.entries + `}`, "provisions.json": `[{"node_id":"tx:1","kind":"dieu","label":"Điều 1","ordinal":1,"path":"dieu-1","text":"Nội dung"}]`})
			key := "vn:vbpl:1"
			tx := &noneTx{}
			l := loader{tx: tx, inv: Inventory{Country: "vn", ExpectedKeys: []string{key}}, artifacts: store, present: map[string]bool{key: true}, docs: map[string]*loadedDocument{}, items: map[string]map[string]InventoryItem{
				"fetch":   {key: {ItemKey: key, State: "done", Path: &f.Path, ArtifactSHA256: &f.SHA256}},
				"extract": {key: {ItemKey: key, State: "done", Path: &e.Path, ArtifactSHA256: &e.SHA256, InputHash: &input}},
			}}
			err := l.loadDocuments(ctx)
			if err == nil || !strings.Contains(err.Error(), "text source none") {
				t.Fatalf("expected none source rejection, got %v", err)
			}
			if tx.inserts != 0 {
				t.Fatalf("wrote %d rows before rejection", tx.inserts)
			}
		})
	}
}
