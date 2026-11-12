package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestAttachment_EmptyFileDoesNotReadSource(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(416) }))
	defer srv.Close()
	s := New(vbpl.NewWithBase(srv.URL, "test", vbpl.WithTestHTTP(), vbpl.WithBackoffs()))
	q := &memoryHashes{rows: map[string]store.FileHash{}}
	dir := t.TempDir()
	d, _, reason, err := s.attachment(context.Background(), "1", dir, vbpl.File{Name: "empty.pdf", Size: 0, URL: srv.URL + "/empty"}, nil, true, q)
	if err != nil || calls.Load() != 0 || reason != "empty_file" {
		t.Fatalf("calls=%d reason=%s error=%v", calls.Load(), reason, err)
	}
	const sha = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	const md5 = "d41d8cd98f00b204e9800998ecf8427e"
	if d.SHA256 != sha || d.Hashes.MD5 != md5 || d.Size != 0 {
		t.Fatalf("empty hashes=%+v", d)
	}
	data, err := os.ReadFile(d.Path)
	if err != nil || len(data) != 0 || filepath.Base(d.Path) != sha {
		t.Fatalf("file=%q bytes=%d error=%v", d.Path, len(data), err)
	}
	if row, ok := q.rows[sha]; !ok || row.Md5 != md5 || row.Etag16 != nil || row.Etag5 != nil {
		t.Fatalf("stored empty hashes=%+v", row)
	}
}
func TestFetch_EmptyAndNoURLCountsAreSeparate(t *testing.T) {
	for _, size := range []int64{0, 3} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			empty := reviewEntry("empty.pdf", "empty", 0)
			missing := reviewEntry("missing.pdf", "missing", int(size))
			missing["presignedUrl"] = ""
			f := &fixtureServer{files: []map[string]any{empty, missing}, bodies: map[string]string{"empty": ""}}
			s, env := setup(t, f)
			out := process(t, s, env)
			counts := parseReadMessage(out.Message).counts
			if counts["empty_file"] != 1 || counts["no_url"] != 1 || counts["etag_unavailable"] != 0 || counts["failure_http_404"] != 0 {
				t.Fatalf("counts=%v", counts)
			}
			if f.downloads.Load() != 0 {
				t.Fatalf("downloads=%d", f.downloads.Load())
			}
			res, err := s.Resolve(context.Background(), env, stage.Run{}, stage.Item{Key: "vn:vbpl:66518"}, *out.Artifact)
			if err != nil || len(res.Gaps) != 1 || res.Gaps[0] != (stage.Gap{Kind: "missing_attachment", Detail: "missing.pdf"}) {
				t.Fatalf("gaps=%+v error=%v", res, err)
			}
			var emptyObservation *stage.FetchObservation
			for i := range out.Observations {
				o := &out.Observations[i]
				if o.Name != nil && *o.Name == "empty.pdf" {
					emptyObservation = o
				}
			}
			if emptyObservation == nil || emptyObservation.Acquisition != "cache" || emptyObservation.FetchedAt != nil || emptyObservation.OriginRef != nil {
				t.Fatalf("empty observation=%+v", emptyObservation)
			}
		})
	}
}
