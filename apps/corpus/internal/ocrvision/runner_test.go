package ocrvision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/ocrcheck"
	"github.com/jackc/pgx/v5/pgtype"
)

type memoryStore struct {
	calls                int
	hash, engine, origin string
	pages                pgtype.Int4
	text                 []byte
	present              bool
}

func (s *memoryStore) Put(_ context.Context, hash, engine, origin string, pages pgtype.Int4, text []byte) (bool, error) {
	s.calls++
	s.hash = hash
	s.engine = engine
	s.origin = origin
	s.pages = pages
	s.text = append([]byte(nil), text...)
	return s.present, nil
}

type memoryRenderer struct {
	pages [][]byte
	calls int
	err   error
}

func (r *memoryRenderer) Preflight(context.Context) error { return nil }
func (r *memoryRenderer) RenderDPI(_ context.Context, _ []byte, _ int, dpi int) (ocrcheck.Rendered, error) {
	r.calls++
	if dpi != 200 {
		return ocrcheck.Rendered{}, fmt.Errorf("wrong dpi")
	}
	return ocrcheck.Rendered{Pages: r.pages}, r.err
}
func document(t *testing.T) Document {
	t.Helper()
	data := []byte("verified PDF")
	path := filepath.Join(t.TempDir(), "pdf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return Document{Hash: fmt.Sprintf("%x", sha256.Sum256(data)), Path: path}
}

func TestRunner_DocumentOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, outcome                     string
		count, rendered, budget           int
		response                          string
		large, hashMismatch, present, dry bool
		requests                          int
		text                              string
	}{
		{name: "joined", outcome: "stored", count: 3, rendered: 3, budget: 3, response: `{"responses":[{"fullTextAnnotation":{"text":"text"}}]}`, requests: 3, text: "text\n\ntext\n\ntext"},
		{name: "empty page", outcome: "stored", count: 3, rendered: 3, budget: 3, response: "ordered", requests: 3, text: "first\n\n\n\nthird"},
		{name: "transient", outcome: "stored", count: 1, rendered: 1, budget: 2, response: "transient", requests: 2, text: "text"},
		{name: "page error", outcome: "failed", count: 1, rendered: 1, budget: 4, response: `{"responses":[{"error":{"code":3}}]}`, requests: 1},
		{name: "empty", outcome: "empty_text", count: 1, rendered: 1, budget: 1, response: `{"responses":[{}]}`, requests: 1},
		{name: "whitespace", outcome: "empty_text", count: 1, rendered: 1, budget: 1, response: `{"responses":[{"fullTextAnnotation":{"text":"  \n"}}]}`, requests: 1},
		{name: "limit", outcome: "skipped_limit", count: 3, rendered: 3, budget: 2},
		{name: "retry budget", outcome: "failed", count: 1, rendered: 1, budget: 1, response: `{"responses":[{"error":{"code":14}}]}`, requests: 1},
		{name: "too many", outcome: "too_many_pages", count: 501, rendered: 1, budget: 1000},
		{name: "large page", outcome: "page_too_large", count: 1, rendered: 1, budget: 1, large: true},
		{name: "mismatch", outcome: "hash_mismatch", count: 1, rendered: 1, budget: 1, hashMismatch: true},
		{name: "render count", outcome: "render_failed", count: 3, rendered: 2, budget: 3},
		{name: "present", outcome: "present", count: 1, rendered: 1, budget: 1, response: `{"responses":[{"fullTextAnnotation":{"text":"text"}}]}`, requests: 1, present: true, text: "text"},
		{name: "dry", count: 3, rendered: 3, budget: 1, dry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			c := fakeClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body := tc.response
				if body == "transient" {
					body = `{"responses":[{"fullTextAnnotation":{"text":"text"}}]}`
					if calls.Load() == 1 {
						body = `{"responses":[{"error":{"code":14}}]}`
					}
				}
				if body == "ordered" {
					data, _ := io.ReadAll(r.Body)
					body = `{"responses":[{}]}`
					if bytes.Contains(data, []byte(`"content":"AQ=="`)) {
						body = `{"responses":[{"fullTextAnnotation":{"text":"first"}}]}`
					}
					if bytes.Contains(data, []byte(`"content":"Aw=="`)) {
						body = `{"responses":[{"fullTextAnnotation":{"text":"third"}}]}`
					}
				}
				_, _ = io.WriteString(w, body)
			})
			s := &memoryStore{present: tc.present}
			render := &memoryRenderer{}
			for i := range tc.rendered {
				render.pages = append(render.pages, []byte{byte(i + 1)})
			}
			if tc.large {
				render.pages[0] = make([]byte, 7_000_001)
			}
			d := document(t)
			if tc.hashMismatch {
				d.Hash = strings.Repeat("a", 64)
			}
			r := Runner{Client: c, Store: s, Renderer: render, CountPages: func(_ context.Context, data []byte) (int, error) {
				if string(data) != "verified PDF" {
					t.Error("wrong counted bytes")
				}
				return tc.count, nil
			}}
			var output strings.Builder
			summary, err := r.Run(context.Background(), []Document{d}, Options{MaxPages: tc.budget, DryRun: tc.dry}, &output)
			if calls.Load() != int64(tc.requests) {
				t.Fatalf("requests=%d want=%d", calls.Load(), tc.requests)
			}
			if tc.dry {
				if err != nil || render.calls != 0 || s.calls != 0 || !strings.Contains(output.String(), fmt.Sprintf("pdf=%s pages=3\n", d.Hash)) {
					t.Fatal("dry run performed work")
				}
				return
			}
			want := fmt.Sprintf("pdf=%s pages=%d outcome=%s requests=%d\n", d.Hash, tc.count, tc.outcome, tc.requests)
			if tc.hashMismatch {
				want = fmt.Sprintf("pdf=%s pages=0 outcome=hash_mismatch requests=0\n", d.Hash)
			}
			if !strings.HasPrefix(output.String(), want) {
				t.Fatalf("output=%q want prefix=%q", output.String(), want)
			}
			if strings.Contains(output.String(), "private-") {
				t.Fatal("output leaked credentials")
			}
			if tc.text != "" {
				if s.calls != 1 || string(s.text) != tc.text || s.engine != EngineID || s.origin != "vision" || !s.pages.Valid || int(s.pages.Int32) != tc.count || s.hash != d.Hash {
					t.Fatalf("store=%+v", s)
				}
			} else if s.calls != 0 {
				t.Fatal("failed document stored")
			}
			failed := tc.outcome != "stored" && tc.outcome != "present" && tc.outcome != "skipped_limit"
			if (err != nil) != failed || (summary.Failed > 0) != failed {
				t.Fatalf("summary=%+v err=%v", summary, err)
			}
		})
	}
}
