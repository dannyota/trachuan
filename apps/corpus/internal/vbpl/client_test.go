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
	return NewWithBase(s.URL, "test", WithTestHTTP(), WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond, time.Millisecond, time.Millisecond))
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
func TestClient_UsesConfiguredBackoffs(t *testing.T) {
	var attempts atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer s.Close()
	c := NewWithBase(s.URL, "test", WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond, time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := c.Detail(ctx, "1")
	if err == nil || errors.Is(err, context.DeadlineExceeded) || attempts.Load() != 3 {
		t.Fatalf("error = %v, attempts = %d", err, attempts.Load())
	}
}
func TestClient_LimitsRate(t *testing.T) {
	b := fixture(t, "detail-67878.json")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(b) }))
	defer s.Close()
	c := NewWithBase(s.URL, "test", WithLimits(50, 8))
	start := time.Now()
	for i := 0; i < 11; i++ {
		if _, _, err := c.Detail(context.Background(), "67878"); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 190*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("11 requests took %v, want 190ms to 2s", elapsed)
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
	c := NewWithBase(s.URL, "test", WithTestHTTP(), WithLimits(float64(rate.Inf), 8), WithBackoffs(time.Millisecond, time.Millisecond, time.Millisecond), WithTestTimeouts(5*time.Millisecond, 50*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	got, err := c.Download(ctx, "1", File{URL: s.URL}, t.TempDir(), 100)
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
		if r.Header.Get("Origin") != "https://vbpl.vn" || r.Header.Get("Referer") != "https://vbpl.vn/" || r.Header.Get("User-Agent") != "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36" {
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

func TestFiles_UpdatedAtVietnamTime(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":[{"updatedAt":"2026-04-14 14:41:19"},{"updatedAt":"bad"},{}]}`))
	}))
	defer s.Close()
	files, err := testClient(s).Files(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 14, 7, 41, 19, 0, time.UTC)
	if len(files) != 3 || !files[0].UpdatedAt.Equal(want) || !files[1].UpdatedAt.IsZero() || !files[2].UpdatedAt.IsZero() {
		t.Fatalf("files = %+v", files)
	}
}

func TestClient_RefusesRedirect(t *testing.T) {
	var reached atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			reached.Store(true)
			return
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer s.Close()
	_, _, err := testClient(s).Detail(context.Background(), "1")
	if err == nil || reached.Load() {
		t.Fatalf("error = %v, reached redirect = %v", err, reached.Load())
	}
}

func TestClient_UsesHTTP11(t *testing.T) {
	var proto string
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proto = r.Proto
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"1"}}`))
	}))
	s.EnableHTTP2 = true
	s.StartTLS()
	defer s.Close()
	c := testClient(s)
	c.http.Transport.(*http.Transport).TLSClientConfig = s.Client().Transport.(*http.Transport).TLSClientConfig
	_, _, err := c.Detail(context.Background(), "1")
	if err != nil || proto != "HTTP/1.1" {
		t.Fatalf("protocol = %q, error = %v", proto, err)
	}
}

func TestDownload_RefusesForeignHost(t *testing.T) {
	var reached atomic.Bool
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer s.Close()
	c := NewWithBase("https://gateway.example", "test")
	c.http.Transport = s.Client().Transport
	_, err := c.Download(context.Background(), "1", File{URL: s.URL}, t.TempDir(), 10)
	if err == nil || errors.Is(err, ErrNotFound) || reached.Load() {
		t.Fatalf("error = %v, reached = %v", err, reached.Load())
	}
}

func TestDownload_EmptyURLIsNotFound(t *testing.T) {
	c := NewWithBase("https://gateway.example", "test")
	_, err := c.Download(context.Background(), "1", File{URL: ""}, t.TempDir(), 10)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestDownload_RefusesPlainHTTP(t *testing.T) {
	var reached atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer s.Close()
	c := NewWithBase(s.URL, "test")
	_, err := c.Download(context.Background(), "1", File{URL: s.URL}, t.TempDir(), 10)
	if err == nil || reached.Load() {
		t.Fatalf("error = %v, reached = %v", err, reached.Load())
	}
}

func TestDownload_SizeMismatchLeavesNoFile(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("short")) }))
	defer s.Close()
	dir := t.TempDir()
	_, err := testClient(s).Download(context.Background(), "1", File{URL: s.URL, Size: 10}, dir, 100)
	entries, readErr := os.ReadDir(dir)
	if err == nil || readErr != nil || len(entries) != 0 {
		t.Fatalf("error = %v, entries = %v, read = %v", err, entries, readErr)
	}
}

func TestClient_PersistentServerErrorIsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := NewWithBase(server.URL, "test", WithLimits(1000, 4), WithBackoffs(time.Millisecond, time.Millisecond))
	_, _, err := client.Detail(context.Background(), "21657")
	var status *StatusError
	if !errors.As(err, &status) || status.Code != http.StatusInternalServerError {
		t.Fatalf("want a StatusError 500, got %v", err)
	}
}

func TestClient_DownloadDoesNotBlockGateway(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("file"))
	}))
	defer fileServer.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"1"}}`))
	}))
	defer gateway.Close()
	c := NewWithBase(gateway.URL, "test", WithTestHTTP(), WithFileHosts(strings.TrimPrefix(fileServer.URL, "http://")), WithLimits(float64(rate.Inf), 1))
	done := make(chan error, 1)
	dir := t.TempDir()
	go func() {
		_, err := c.Download(context.Background(), "1", File{URL: fileServer.URL}, dir, 100)
		done <- err
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, _, err := c.Detail(ctx, "1")
	close(release)
	if downloadErr := <-done; downloadErr != nil {
		t.Fatal(downloadErr)
	}
	if err != nil {
		t.Fatalf("gateway blocked by download: %v", err)
	}
}

func TestClient_UserAgentAllRequests(t *testing.T) {
	const want = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"
	paths := map[string]string{"/doc/all": "list-agencies-62-908-page1.json", "/doc/67878": "detail-67878.json", "/doc/provision/tree/67878": "tree-67878.json", "/doc/67878/diagram": "diagram-67878.json", "/doc/minio/buckets/vbpl/folders/67878/files": "files-67878.json"}
	seen := map[string]bool{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = true
		if got := r.Header.Get("User-Agent"); got != want {
			t.Errorf("%s: User-Agent = %q", r.URL.Path, got)
		}
		if r.URL.Path == "/file" {
			_, _ = w.Write([]byte("file"))
			return
		}
		name, ok := paths[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(fixture(t, name))
	}))
	defer s.Close()
	c := testClient(s)
	ctx := context.Background()
	if _, err := c.List(ctx, ListQuery{Page: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Detail(ctx, "67878"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Tree(ctx, "67878"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Diagram(ctx, "67878"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Files(ctx, "67878"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Download(ctx, "67878", File{URL: s.URL + "/file"}, t.TempDir(), 100); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(paths)+1 {
		t.Fatalf("requests seen: %v", seen)
	}
}
