package fetch

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestAttachment_HashOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, etag, reason                   string
		status                               int
		cache, duplicate, wrongSize, corrupt bool
	}{
		{name: "baseline MD5", reason: "etag_md5_baseline", status: 206},
		{name: "cache MD5", reason: "etag_md5_cache", status: 206, cache: true},
		{name: "different", etag: strings.Repeat("0", 32), reason: "etag_differs", status: 206},
		{name: "unexplained multipart", etag: strings.Repeat("0", 32) + "-2", reason: "etag_differs", status: 206},
		{name: "other form", etag: "opaque", reason: "etag_other_form", status: 206},
		{name: "missing", etag: "-", reason: "etag_unavailable", status: 206},
		{name: "non partial", reason: "etag_unavailable", status: 200},
		{name: "range failed", reason: "etag_unavailable", status: 500},
		{name: "size mismatch", reason: "no_candidate", status: 206, wrongSize: true},
		{name: "duplicate", reason: "no_candidate", status: 206, duplicate: true},
		{name: "corrupted", reason: "candidate_unverified", status: 206, corrupt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const body = "one"
			var downloads, ranges atomic.Int32
			etag := tc.etag
			if etag == "" {
				etag = fmt.Sprintf("%x", md5.Sum([]byte(body)))
			}
			if etag == "-" {
				etag = ""
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "" {
					ranges.Add(1)
					if r.Header.Get("Range") != "bytes=0-0" {
						t.Error("wrong range")
					}
					w.Header().Set("ETag", `"`+etag+`"`)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte("o"))
					return
				}
				downloads.Add(1)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			s := New(vbpl.NewWithBase(srv.URL, "test", vbpl.WithTestHTTP(), vbpl.WithBackoffs()))
			b := &baseline{dir: t.TempDir(), files: []file{{Name: "a", SHA256: digest([]byte(body)), Size: 3}}}
			if err := os.Mkdir(filepath.Join(b.dir, "files"), 0700); err != nil {
				t.Fatal(err)
			}
			bytes := body
			if tc.corrupt {
				bytes = "bad"
			}
			if err := os.WriteFile(filepath.Join(b.dir, "files", b.files[0].SHA256), []byte(bytes), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.wrongSize {
				b.files[0].Size = 4
			}
			if err := b.indexOrigins(context.Background()); err != nil {
				t.Fatal(err)
			}
			if tc.cache {
				s.WithCache(cacheFixture(t, body))
				b = nil
			}
			d, _, reason, err := s.attachment(context.Background(), "66518", t.TempDir(), vbpl.File{Name: "a", Size: 3, URL: srv.URL + "/a"}, b, !tc.duplicate, nil)
			want := int32(1)
			if strings.HasPrefix(tc.reason, "etag_md5_") {
				want = 0
			}
			if err != nil || reason != tc.reason || downloads.Load() != want || ranges.Load() != 1 || d.SHA256 != digest([]byte(body)) {
				t.Fatalf("reason=%s downloads=%d ranges=%d output=%+v err=%v", reason, downloads.Load(), ranges.Load(), d, err)
			}
		})
	}
}

func TestAttachment_ETag403Pauses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) }))
	defer srv.Close()
	gate := sourcegate.NewMemory()
	c := vbpl.NewWithBase(srv.URL, "test", vbpl.WithTestHTTP(), vbpl.WithGate(gate))
	ctx := sourcegate.WithActivity(sourcegate.WithRunID(context.Background(), 1))
	_, err := c.ETag(ctx, srv.URL+"/a")
	if !strings.Contains(fmt.Sprint(err), "pause") {
		t.Fatal(err)
	}
}

func TestAttachment_MultipartCopies(t *testing.T) {
	for _, part := range []int{5 << 20, 16 << 20} {
		t.Run(fmt.Sprint(part), func(t *testing.T) {
			data := []byte(strings.Repeat("x", part+1))
			h := filecache.NewHasher()
			_, _ = h.Write(data)
			hashes := h.Hashes()
			etag := hashes.ETag5
			if part == 16<<20 {
				etag = hashes.ETag16
			}
			dir := t.TempDir()
			path := filepath.Join(dir, hashes.SHA256)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			d, reason, err := copyCandidate(context.Background(), candidate{path: path, hash: hashes.SHA256, size: int64(len(data)), source: "baseline"}, t.TempDir(), etag, nil)
			if err != nil || reason != "etag_multipart_baseline" || d.SHA256 != hashes.SHA256 {
				t.Fatalf("%+v %s %v", d, reason, err)
			}
		})
	}
}

func TestAttachment_BaselineThenCache(t *testing.T) {
	for _, body := range []string{"one", "two"} {
		t.Run(body, func(t *testing.T) {
			f := &fixtureServer{files: []map[string]any{reviewEntry("a", "a", 3)}, bodies: map[string]string{"a": body}}
			s, env := setup(t, f)
			first := process(t, s, env)
			b := baselineFromOutput(t, env, first)
			s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
			s.WithCache(cacheFixture(t, "one"))
			f.bodies["a"] = "one"
			f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null}}`
			f.downloads.Store(0)
			out := process(t, s, env)
			want := "etag_md5_baseline"
			if body == "two" {
				want = "etag_md5_cache"
			}
			if f.downloads.Load() != 0 || parseReadMessage(out.Message).counts[want] != 1 {
				t.Fatalf("downloads=%d message=%s", f.downloads.Load(), out.Message)
			}
		})
	}
}
func TestAttachment_CopiedBundleEqualsDownloaded(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{reviewEntry("a", "a", 3)}, bodies: map[string]string{"a": "one"}, detailBody: `{"success":true,"data":{"id":"66518","updatedDate":null}}`}
	s, env := setup(t, f)
	first := process(t, s, env)
	b := baselineFromOutput(t, env, first)
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
	f.downloads.Store(0)
	second := process(t, s, env)
	if first.Artifact.SHA256 != second.Artifact.SHA256 || f.downloads.Load() != 0 || parseReadMessage(second.Message).counts["etag_md5_baseline"] != 1 {
		t.Fatalf("downloads=%d message=%s", f.downloads.Load(), second.Message)
	}
}
