//go:build integration

package ocrvision

import (
	"context"
	"io"
	"net/http"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVision_StoreFreshEntry(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := ocrstore.Open(t.TempDir(), db)
	c := fakeClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"responses":[{"fullTextAnnotation":{"text":"public text"}}]}`)
	})
	d := document(t)
	runner := Runner{Client: c, Store: source, Renderer: &memoryRenderer{pages: [][]byte{{1}, {2}, {3}}}, CountPages: func(context.Context, []byte) (int, error) { return 3, nil }}
	summary, err := runner.Run(ctx, []Document{d}, Options{MaxPages: 3}, io.Discard)
	if err != nil || summary.Stored != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	entry, err := store.New(db).GetOCRText(ctx, store.GetOCRTextParams{PdfSha256: d.Hash, EngineID: EngineID})
	if err != nil || entry.Origin != "vision" || !entry.Pages.Valid || entry.Pages.Int32 != 3 {
		t.Fatalf("entry=%+v err=%v", entry, err)
	}
	text, err := source.ReadText(entry)
	if err != nil || string(text) != "public text\n\npublic text\n\npublic text" {
		t.Fatalf("text=%q err=%v", text, err)
	}
	summary, err = runner.Run(ctx, []Document{d}, Options{MaxPages: 3}, io.Discard)
	if err != nil || summary.Present != 1 {
		t.Fatalf("repeat summary=%+v err=%v", summary, err)
	}
}
