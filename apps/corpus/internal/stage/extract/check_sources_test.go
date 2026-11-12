package extract

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type engineCheckSource struct {
	entries []ocrstore.Entry
	data    []byte
}

func (f engineCheckSource) ReadText(store.OcrText) ([]byte, error) { return f.data, nil }
func (f engineCheckSource) Lookup(_ context.Context, hashes, engines []string, _ string) (map[string]ocrstore.Entry, error) {
	out := map[string]ocrstore.Entry{}
	for _, hash := range hashes {
		for _, engine := range engines {
			for _, e := range f.entries {
				if e.Text.PdfSha256 == hash && e.Text.EngineID == engine {
					if _, ok := out[hash]; !ok {
						out[hash] = e
					}
				}
			}
		}
	}
	return out, nil
}
func TestCheck_AllEngineEntries(t *testing.T) {
	base := ocrFixture(t, false, 0)
	hash := sourceHash(base)
	a := base.entries[hash]
	a.Text.ID = 1
	b := a
	b.Text.ID = 2
	b.Text.EngineID = "vision-ocr/builtin-latest/poppler-200-jpeg"
	source := engineCheckSource{entries: []ocrstore.Entry{a, b}, data: base.data}
	dir := headingBundle(t, `{"success":true,"data":[]}`, false)
	if err := os.WriteFile(filepath.Join(dir, "files.json"), []byte(`[{"name":"a.pdf","sha256":"`+hash+`","size":20}]`), 0600); err != nil {
		t.Fatal(err)
	}
	engines := []string{a.Text.EngineID, b.Text.EngineID}
	one, err := SelectCheckSource(context.Background(), dir, "vn:vbpl:1", source, engines, "checker")
	if err != nil || one == nil || one.Entry.ID != 1 {
		t.Fatalf("preferred=%+v err=%v", one, err)
	}
	all, err := SelectCheckSources(context.Background(), dir, "vn:vbpl:1", source, engines, "checker")
	if err != nil || len(all) != 2 || all[0].Entry.ID != 1 || all[1].Entry.ID != 2 {
		t.Fatalf("all=%+v err=%v", all, err)
	}
}
