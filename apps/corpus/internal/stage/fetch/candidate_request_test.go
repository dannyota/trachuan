package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestAttachment_NoCandidateUsesOneDownload(t *testing.T) {
	for _, mode := range []string{"no baseline", "duplicate name", "baseline size mismatch", "cache size mismatch"} {
		t.Run(mode, func(t *testing.T) {
			body := "one"
			size := int64(3)
			if mode == "cache size mismatch" {
				body = "four"
				size = 4
			}
			var ranges, downloads atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "" {
					ranges.Add(1)
					w.WriteHeader(403)
					return
				}
				downloads.Add(1)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			s := New(vbpl.NewWithBase(srv.URL, "test", vbpl.WithTestHTTP(), vbpl.WithBackoffs()))
			var b *baseline
			unique := true
			switch mode {
			case "duplicate name", "baseline size mismatch":
				b = &baseline{files: []file{{Name: "a", SHA256: digest([]byte("one")), Size: 3}}}
				if mode == "duplicate name" {
					unique = false
				} else {
					b.files[0].Size = 4
				}
				if err := b.indexOrigins(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "cache size mismatch":
				s.WithCache(cacheFixture(t, "one"))
			}
			got, _, reason, err := s.attachment(context.Background(), "66518", t.TempDir(), vbpl.File{Name: "a", Size: size, URL: srv.URL + "/a"}, b, unique, nil)
			if err != nil || reason != "no_candidate" || ranges.Load() != 0 || downloads.Load() != 1 || got.SHA256 != digest([]byte(body)) {
				t.Fatalf("reason=%s ranges=%d downloads=%d output=%+v error=%v", reason, ranges.Load(), downloads.Load(), got, err)
			}
		})
	}
}
