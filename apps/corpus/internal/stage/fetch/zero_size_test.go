package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestAttachment_ZeroListedSizeReadsSource(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		cause      string
	}{{"empty", "", 200, ""}, {"nonempty", "x", 200, "size"}, {"missing", "", 404, "missing"}, {"paused", "", 403, "pause"}} {
		t.Run(tc.name, func(t *testing.T) {
			var calls, ranges atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Range") != "" {
					ranges.Add(1)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			s := New(vbpl.NewWithBase(srv.URL, "test", vbpl.WithTestHTTP(), vbpl.WithBackoffs()))
			q := &memoryHashes{rows: map[string]store.FileHash{}}
			d, _, reason, err := s.attachment(context.Background(), "1", t.TempDir(), vbpl.File{Name: "empty", Size: 0, URL: srv.URL}, nil, true, q)
			if calls.Load() != 1 || ranges.Load() != 0 || reason != "no_candidate" {
				t.Fatalf("calls=%d ranges=%d reason=%s error=%v", calls.Load(), ranges.Load(), reason, err)
			}
			switch tc.cause {
			case "":
				if err != nil || d.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || d.Hashes.MD5 != "d41d8cd98f00b204e9800998ecf8427e" || len(q.rows) != 1 {
					t.Fatalf("download=%+v hashes=%v error=%v", d, q.rows, err)
				}
			case "size":
				var f *vbpl.Failure
				if !errors.As(err, &f) || f.Cause != "size" || len(q.rows) != 0 {
					t.Fatalf("failure=%+v rows=%v error=%v", f, q.rows, err)
				}
			case "missing":
				if !errors.Is(err, vbpl.ErrNotFound) {
					t.Fatal(err)
				}
			case "pause":
				if !errors.Is(err, sourcegate.ErrSourcePaused) {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestFetch_ZeroSizeAndNoURLCountsAreSeparate(t *testing.T) {
	for _, size := range []int64{0, 3} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			empty := reviewEntry("empty.pdf", "empty", 0)
			missing := reviewEntry("missing.pdf", "missing", int(size))
			missing["presignedUrl"] = ""
			f := &fixtureServer{files: []map[string]any{empty, missing}, bodies: map[string]string{"empty": ""}}
			s, env := setup(t, f)
			out := process(t, s, env)
			counts := parseReadMessage(out.Message).counts
			if counts["no_candidate"] != 1 || counts["no_url"] != 1 || counts["etag_unavailable"] != 0 || counts["failure_http_404"] != 0 {
				t.Fatalf("counts=%v", counts)
			}
			if f.downloads.Load() != 1 {
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
			if emptyObservation == nil || emptyObservation.Acquisition != "network" || emptyObservation.FetchedAt == nil || emptyObservation.OriginRef != nil {
				t.Fatalf("empty observation=%+v", emptyObservation)
			}
		})
	}
}
