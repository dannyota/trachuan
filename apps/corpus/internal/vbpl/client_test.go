package vbpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func testClient(s *httptest.Server) *Client {
	c := New("test")
	c.base = s.URL
	c.limiter = rate.NewLimiter(rate.Inf, 1)
	c.backoffs = []time.Duration{0, 0, 0}
	return c
}
func TestClient_RetriesOn503ThenSucceeds(t *testing.T) {
	var n atomic.Int32
	b := fixture(t, "detail-67878.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write(b)
	}))
	defer s.Close()
	_, d, err := testClient(s).Detail(context.Background(), "67878")
	if err != nil || d.ID != "67878" || n.Load() != 2 {
		t.Fatalf("detail=%+v attempts=%d err=%v", d, n.Load(), err)
	}
}
func TestClient_LimitsRate(t *testing.T) {
	b := fixture(t, "detail-67878.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(b) }))
	defer s.Close()
	c := testClient(s)
	c.limiter = rate.NewLimiter(2, 1)
	start := time.Now()
	for i := 0; i < 10; i++ {
		if _, _, err := c.Detail(context.Background(), "67878"); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) < 4500*time.Millisecond {
		t.Fatal("rate exceeded")
	}
}
func TestClient_ResponseOverLimit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(make([]byte, 20_000_001)) }))
	defer s.Close()
	_, _, err := testClient(s).Detail(context.Background(), "1")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}
func TestDecodeDetail_NullObjects(t *testing.T) {
	b := fixture(t, "detail-245a9a00-d447-11f1-84e4-cfbf13844ed1.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(b) }))
	defer s.Close()
	_, d, err := testClient(s).Detail(context.Background(), "x")
	if err != nil || d.StatusCode != "" || d.ID != "245a9a00-d447-11f1-84e4-cfbf13844ed1" {
		t.Fatalf("detail=%+v err=%v", d, err)
	}
}
func TestDecodeTree_NestedNodes(t *testing.T) {
	b := fixture(t, "tree-67878.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(b) }))
	defer s.Close()
	_, nodes, err := testClient(s).Tree(context.Background(), "67878")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	var walk func([]Node)
	walk = func(ns []Node) {
		for _, n := range ns {
			counts[n.Level]++
			walk(n.Children)
		}
	}
	walk(nodes)
	if counts["Article"] != 11 || counts["Clause"] != 34 || counts["Point"] != 17 {
		t.Fatalf("counts %v", counts)
	}
}
func TestDownload_RefreshesExpiredURL(t *testing.T) {
	var s *httptest.Server
	var lists atomic.Int32
	s = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old":
			w.WriteHeader(403)
		case "/new":
			_, _ = w.Write([]byte("complete"))
		default:
			lists.Add(1)
			_, _ = fmt.Fprintf(w, `{"success":true,"data":[{"fullPath":"a","presignedUrl":%q}]}`, s.URL+"/new")
		}
	}))
	defer s.Close()
	got, err := testClient(s).Download(context.Background(), "1", File{FullPath: "a", URL: s.URL + "/old"}, t.TempDir(), 100)
	if err != nil || got.Size != 8 || lists.Load() != 1 {
		t.Fatalf("got %+v lists %d err %v", got, lists.Load(), err)
	}
}
func TestDownload_TimeoutAfterPartialBodyRetriesFresh(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			_, _ = w.Write([]byte("part"))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("complete"))
	}))
	defer s.Close()
	c := testClient(s)
	c.timeout = 50 * time.Millisecond
	got, err := c.Download(context.Background(), "1", File{URL: s.URL}, t.TempDir(), 100)
	sum := sha256.Sum256([]byte("complete"))
	if err != nil || got.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("got %+v err %v", got, err)
	}
}
func TestDownload_OverLimit(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("12345")) }))
	defer s.Close()
	dir := t.TempDir()
	_, err := testClient(s).Download(context.Background(), "1", File{URL: s.URL}, dir, 4)
	entries, e := os.ReadDir(dir)
	if !errors.Is(err, ErrTooLarge) || e != nil || len(entries) != 0 {
		t.Fatalf("err=%v entries=%v read=%v", err, entries, e)
	}
}
func TestClient_NeverLogsSignedQuery(t *testing.T) {
	c := New("test")
	_, err := c.Download(context.Background(), "1", File{URL: "http://127.0.0.1:1/file?secret=token"}, t.TempDir(), 10)
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token") {
		t.Fatalf("error %v", err)
	}
}

func TestClient_ListAndFiles(t *testing.T) {
	listing := fixture(t, "list-agencies-62-908-page1.json")
	files := fixture(t, "files-67878.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "https://vbpl.vn" || r.Header.Get("Referer") != "https://vbpl.vn/" || r.Header.Get("User-Agent") != "traquydinh/test (+https://github.com/dannyota/traquydinh)" {
			t.Error("missing gateway headers")
		}
		switch r.URL.Path {
		case "/doc/all":
			var request struct {
				PageSize  int      `json:"pageSize"`
				AgencyIDs []string `json:"agencyIds"`
				MatchMode string   `json:"matchMode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.PageSize != 50 || len(request.AgencyIDs) != 2 || request.MatchMode != "all_words" {
				t.Errorf("invalid listing query: %+v, %v", request, err)
			}
			_, _ = w.Write(listing)
		case "/doc/minio/buckets/vbpl/folders/67878/files":
			if r.URL.RawQuery != "" {
				t.Error("unexpected files query")
			}
			_, _ = w.Write(files)
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	c := testClient(s)
	page, err := c.List(context.Background(), ListQuery{AgencyIDs: []string{"62", "908"}, Page: 1})
	if err != nil || len(page.Items) != 5 || page.Total < 5 || len(page.Items[0].Raw) == 0 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	attachments, err := c.Files(context.Background(), "67878")
	if err != nil || len(attachments) != 5 || attachments[0].FullPath == "" || attachments[0].URL == "" {
		t.Fatalf("files=%+v err=%v", attachments, err)
	}
}

func TestClient_NotFound(t *testing.T) {
	for _, status := range []int{200, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status == 200 {
					_, _ = w.Write([]byte(`{"success":false}`))
				}
			}))
			defer s.Close()
			_, _, err := testClient(s).Detail(context.Background(), "missing")
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
