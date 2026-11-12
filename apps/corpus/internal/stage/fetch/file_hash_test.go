package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"github.com/jackc/pgx/v5"
)

type memoryHashes struct {
	rows   map[string]store.FileHash
	writes int
}

func (m *memoryHashes) GetFileHash(_ context.Context, key string) (store.FileHash, error) {
	v, ok := m.rows[key]
	if !ok {
		return v, pgx.ErrNoRows
	}
	return v, nil
}
func (m *memoryHashes) UpsertFileHash(_ context.Context, p store.UpsertFileHashParams) error {
	m.rows[p.Sha256] = store.FileHash(p)
	m.writes++
	return nil
}

func TestFileHash_FillsAndCorrectsRows(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(fmt.Sprint(wrong), func(t *testing.T) {
			h := filecache.NewHasher()
			_, _ = h.Write([]byte("one"))
			hashes := h.Hashes()
			q := &memoryHashes{rows: map[string]store.FileHash{}}
			if wrong {
				bad := "bad-2"
				q.rows[hashes.SHA256] = store.FileHash{Sha256: hashes.SHA256, Md5: hashes.MD5, Etag16: &bad}
			}
			downloads := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "" {
					w.Header().Set("ETag", `"`+hashes.MD5+`"`)
					w.WriteHeader(206)
					_, _ = w.Write([]byte("o"))
					return
				}
				downloads++
				_, _ = w.Write([]byte("one"))
			}))
			defer srv.Close()
			s := New(vbpl.NewWithBase(srv.URL, "test", vbpl.WithTestHTTP(), vbpl.WithBackoffs()))
			b := &baseline{dir: t.TempDir(), files: []file{{Name: "a", SHA256: hashes.SHA256, Size: 3}}}
			if err := os.Mkdir(filepath.Join(b.dir, "files"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(b.dir, "files", hashes.SHA256), []byte("one"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := b.indexOrigins(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, _, reason, err := s.attachment(context.Background(), "1", t.TempDir(), vbpl.File{Name: "a", Size: 3, URL: srv.URL}, b, true, q)
			want := 0
			if wrong {
				want = 1
			}
			if err != nil || downloads != want || q.writes == 0 {
				t.Fatalf("downloads=%d writes=%d reason=%s err=%v", downloads, q.writes, reason, err)
			}
			row, ok, err := storedHashes(context.Background(), q, hashes.SHA256)
			if err != nil || !ok || row != hashes {
				t.Fatalf("row=%+v err=%v", row, err)
			}
		})
	}
}
