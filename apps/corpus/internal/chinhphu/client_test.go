package chinhphu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func clientFor(t *testing.T, handler http.HandlerFunc, opts ...Option) (*Client, string) {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	base := []Option{WithBaseURLs(s.URL, s.URL), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1, Pages: 100, HTMLBytes: 5000000, FileBytes: 50000000, Redirects: 3}), WithBackoffs(), WithDownloadBudget(1000000000)}
	return New(append(base, opts...)...), s.URL
}
func pages(t *testing.T, names ...string) http.HandlerFunc {
	t.Helper()
	n := 0
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write(fixture(t, "home.html"))
			return
		}
		if e := r.ParseForm(); e != nil {
			t.Error(e)
		}
		if r.Form.Get("ctrl_191017_163$drdRecordPerPage") != "500" {
			t.Error("missing result cap")
		}
		for _, key := range []string{"drdDocOrg", "drdDocCategory", "drdDocYear"} {
			if r.Form.Get("ctrl_191017_163$"+key) != "0" {
				t.Errorf("missing default %s", key)
			}
		}
		if n >= len(names) {
			t.Error("unexpected page")
			w.WriteHeader(500)
			return
		}
		if n > 0 && r.Form.Get("__EVENTARGUMENT") != fmt.Sprintf("Page$%d", n+1) {
			t.Error("wrong page postback")
		}
		_, _ = w.Write(fixture(t, names[n]))
		n++
	}
}
func TestSearch_EmptyAndMultiple(t *testing.T) {
	for _, x := range []struct {
		name  string
		count int
	}{{"search-empty.html", 0}, {"search-number-367-QD-TTg.html", 18}} {
		t.Run(x.name, func(t *testing.T) {
			c, _ := clientFor(t, pages(t, x.name))
			r, b, e := c.Search(context.Background(), "367/QĐ-TTg")
			if e != nil || len(r.Items) != x.count || r.Evidence.Completeness != lookup.Complete || len(b) != 1 {
				t.Fatalf("items=%d completeness=%s bodies=%d err=%v", len(r.Items), r.Evidence.Completeness, len(b), e)
			}
		})
	}
}
func TestSearch_EmptyMarkerIsCompleteZero(t *testing.T) {
	c, _ := clientFor(t, pages(t, "search-empty.html"))
	r, _, e := c.Search(context.Background(), "nothing")
	if e != nil || !r.ProvesAbsence() {
		t.Fatalf("result=%+v err=%v", r, e)
	}
}
func TestSearch_PaginatesAndDedupes(t *testing.T) {
	c, _ := clientFor(t, pages(t, "search-truncated-page1.html", "search-truncated-page2.html", "search-truncated-page3-stale-tokens.html", "search-truncated-page3-stale-tokens.html"))
	r, _, e := c.Search(context.Background(), "ngân hàng")
	if e != nil || len(r.Items) != 150 || len(r.Evidence.PagesFetched) != 4 || r.Evidence.Completeness != lookup.Incomplete {
		t.Fatalf("items=%d pages=%d completeness=%s err=%v", len(r.Items), len(r.Evidence.PagesFetched), r.Evidence.Completeness, e)
	}
}
func TestSearch_CapEqualIsIncomplete(t *testing.T) {
	c, _ := clientFor(t, pages(t, "search-truncated-page1.html", "search-truncated-page2.html", "search-truncated-page3-stale-tokens.html"), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1, Pages: 3, HTMLBytes: 5000000, FileBytes: 50000000, Redirects: 3}))
	r, _, _ := c.Search(context.Background(), "ngân hàng")
	if r.Evidence.Completeness != lookup.Incomplete {
		t.Fatalf("%+v", r.Evidence)
	}
}
func TestSearch_ProvincialRawNumber(t *testing.T) {
	c, _ := clientFor(t, pages(t, "search-provincial-org-16134.html"))
	r, _, e := c.Search(context.Background(), "63")
	if e != nil || len(r.Items) != 1 || r.Items[0].RawNumber == nil || *r.Items[0].RawNumber != "63/2025/QĐ-CTUBND " {
		t.Fatalf("%+v %v", r.Items, e)
	}
}
func TestDetail_ParsesCentralIssuerAndAttachments(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "detail-218029.html")) })
	d, b, e := c.Detail(context.Background(), 218029)
	if e != nil || d.Number != "142/2026/NĐ-CP" || d.Issuer != "Chính phủ" || d.IssueDate != "30-04-2026" || len(d.Attachments) != 1 || len(b) == 0 {
		t.Fatalf("%+v %v", d, e)
	}
}
func TestDetail_ProvincialIssuer(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "detail-215941.html")) })
	d, _, e := c.Detail(context.Background(), 215941)
	if e != nil || d.Issuer != "Chủ tịch UBND tỉnh Phú Thọ" {
		t.Fatalf("%+v %v", d, e)
	}
}
func TestDetail_MissingIsNotFound(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "detail-missing.html")) })
	_, _, e := c.Detail(context.Background(), 999999999)
	if !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
}
func TestDownload_HostAllowlistAndPDFSignature(t *testing.T) {
	c, base := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.pdf" {
			w.WriteHeader(404)
			_, _ = w.Write(fixture(t, "file-missing.xml"))
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		if r.URL.Path == "/good.pdf" {
			_, _ = w.Write(fixture(t, "pdf-142-head.bin"))
			return
		}
		_, _ = io.WriteString(w, "not pdf")
	})
	var dst bytes.Buffer
	info, e := c.Download(context.Background(), base+"/good.pdf", &dst, 2000)
	if e != nil || info.Size != 1024 || info.SHA256 != "4da640e9ddac2ccd1390fa122fc236aeb89da66ac4f28bfe60ff201ae86de8cf" || info.MediaType != "application/pdf" || info.FetchedAt.IsZero() {
		t.Fatalf("%+v %v", info, e)
	}
	if _, e = c.Download(context.Background(), base+"/bad.pdf", io.Discard, 2000); e == nil {
		t.Fatal("accepted invalid PDF")
	}
	if _, e = c.Download(context.Background(), "https://example.com/a.pdf", io.Discard, 2000); e == nil {
		t.Fatal("accepted foreign host")
	}
	if _, e = c.Download(context.Background(), base+"/missing.pdf", io.Discard, 2000); !errors.Is(e, ErrNotFound) {
		t.Fatalf("missing file: %v", e)
	}
}
func TestClient_NoTokenInArtifacts(t *testing.T) {
	const token = "SENSITIVE_TEST_TOKEN"
	const cookie = "SENSITIVE_TEST_COOKIE"
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "ASP.NET_SessionId="+cookie)
		name := "home.html"
		if r.Method == http.MethodPost {
			name = "search-ai.html"
		}
		b := bytes.ReplaceAll(fixture(t, name), []byte("REDACTED-__VIEWSTATE"), []byte(token))
		_, _ = w.Write(b)
	})
	r, b, e := c.Search(context.Background(), "none")
	if e != nil || len(b) != 1 || len(r.Items) != 11 {
		t.Fatalf("result=%d err=%v", len(r.Items), e)
	}
	data, _ := json.Marshal(r)
	sum := sha256.Sum256(b[0])
	if r.Evidence.PagesFetched[0].SHA256 != fmt.Sprintf("%x", sum) {
		t.Fatal("hash does not cover redacted body")
	}
	all := string(data) + string(bytes.Join(b, nil)) + fmt.Sprint(e)
	if strings.Contains(all, token) || strings.Contains(all, cookie) {
		t.Fatal("secret in returned body or evidence")
	}
}

func TestClient_ChallengePausesSource(t *testing.T) {
	g := sourcegate.NewMemory()
	var n atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = io.WriteString(w, "<html>captcha challenge-platform</html>")
	}
	c, base := clientFor(t, handler, WithGate(g))
	ctx := sourcegate.WithRunID(context.Background(), 1)
	_, _, e := c.Search(ctx, "a")
	if !errors.Is(e, sourcegate.ErrSourcePaused) {
		t.Fatal(e)
	}
	other := New(WithBaseURLs(base, base), WithGate(g))
	_, _, e = other.Search(ctx, "b")
	if !errors.Is(e, sourcegate.ErrSourcePaused) || n.Load() != 1 {
		t.Fatalf("requests=%d err=%v", n.Load(), e)
	}
}
func TestClient_RedirectToForeignHostRejected(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/", http.StatusFound)
	})
	_, _, e := c.Search(context.Background(), "x")
	if e == nil {
		t.Fatal("accepted redirect")
	}
}
func TestClient_HTTPToHTTPSRedirect(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "detail-missing.html")) }))
	defer s.Close()
	c, base := clientFor(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, s.URL, http.StatusFound) }, WithTransport(s.Client().Transport))
	c.testHosts[s.URL] = true
	_, _, e := c.Detail(context.Background(), 1)
	if !errors.Is(e, ErrNotFound) {
		t.Fatalf("base=%s err=%v", base, e)
	}
}
func TestClient_NonstandardPortRejected(t *testing.T) {
	c := New()
	_, e := c.Download(context.Background(), "https://datafiles.chinhphu.vn:444/x", io.Discard, 100)
	if e == nil {
		t.Fatal("accepted nonstandard port")
	}
}
func TestClient_RetryAfterDeferred(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "601"); w.WriteHeader(429) }, WithBackoffs(time.Millisecond))
	_, _, e := c.Search(context.Background(), "x")
	if !errors.Is(e, ErrDeferred) {
		t.Fatal(e)
	}
}
func TestClient_OversizeHTML(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 100)) }, WithLimits(Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1, Pages: 100, HTMLBytes: 20, FileBytes: 1000, Redirects: 3}))
	_, _, e := c.Search(context.Background(), "x")
	if !errors.Is(e, ErrTooLarge) {
		t.Fatal(e)
	}
}

func TestDownload_ChallengePausesSource(t *testing.T) {
	g := sourcegate.NewMemory()
	c, base := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html>cf-chl captcha</html>")
	}, WithGate(g))
	_, e := c.Download(sourcegate.WithRunID(context.Background(), 7), base+"/file", io.Discard, 1000)
	if !errors.Is(e, sourcegate.ErrSourcePaused) {
		t.Fatal(e)
	}
}
func TestDownload_InvalidPayloadClassified(t *testing.T) {
	c, base := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "not pdf")
	})
	_, e := c.Download(context.Background(), base+"/bad", io.Discard, 100)
	var f *Failure
	if !errors.As(e, &f) || f.Cause != "invalid_payload" {
		t.Fatalf("%v", e)
	}
}
func TestClient_DownloadBudgetCountsRejectedBodies(t *testing.T) {
	var n atomic.Int32
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "12345678")
	}, WithBackoffs(0), WithDownloadBudget(10))
	_, _, e := c.Search(context.Background(), "x")
	if !errors.Is(e, ErrBudget) || n.Load() != 2 {
		t.Fatalf("requests=%d error=%v", n.Load(), e)
	}
}
func TestClient_RetriesAndHonorsRetryAfter(t *testing.T) {
	var n atomic.Int32
	var delays []time.Duration
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
			return
		}
		_, _ = w.Write(fixture(t, "detail-missing.html"))
	}, WithBackoffs(time.Second), WithClock(time.Now, func(_ context.Context, d time.Duration) error { delays = append(delays, d); return nil }))
	_, _, e := c.Detail(context.Background(), 1)
	if !errors.Is(e, ErrNotFound) || len(delays) != 1 || delays[0] != 3*time.Second {
		t.Fatalf("delays=%v err=%v", delays, e)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClient_RejectsNonpublicResolvedAddress(t *testing.T) {
	c := New(WithResolver(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("100.64.0.1")}}, nil
	}), WithTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("attempted transport for nonpublic address")
		return nil, errors.New("blocked test transport")
	})))
	_, e := c.Download(context.Background(), "https://datafiles.chinhphu.vn/a", io.Discard, 100)
	if e == nil {
		t.Fatal("accepted nonpublic address")
	}
	var f *Failure
	if !errors.As(e, &f) || f.Cause != "invalid_payload" {
		t.Fatalf("%v", e)
	}
}
func TestSearch_CompleteMultiPageWithStaleTokens(t *testing.T) {
	var n int
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = w.Write(fixture(t, "home.html"))
			return
		}
		names := []string{"search-truncated-page1.html", "search-truncated-page2.html", "search-truncated-page3-stale-tokens.html"}
		b := fixture(t, names[n])
		n++
		b = bytes.Replace(b, []byte("| 500"), []byte("| 150"), 1)
		_, _ = w.Write(b)
	})
	r, _, e := c.Search(context.Background(), "bank")
	if e != nil || len(r.Items) != 150 || r.Evidence.Completeness != lookup.Complete {
		t.Fatalf("items=%d completeness=%s err=%v", len(r.Items), r.Evidence.Completeness, e)
	}
}

func TestDownload_LimitDoesNotWriteExcess(t *testing.T) {
	c, base := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-123456")
	})
	var b bytes.Buffer
	_, e := c.Download(context.Background(), base+"/x", &b, 6)
	if !errors.Is(e, ErrTooLarge) || b.Len() > 6 {
		t.Fatalf("bytes=%d err=%v", b.Len(), e)
	}
}
func TestDetail_InvalidAttachmentFails(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		b := fixture(t, "detail-218029.html")
		b = bytes.ReplaceAll(b, []byte("https://datafiles.chinhphu.vn/cpp/files/vbpq/2026/4/142-2026-ndcp.signed.pdf"), []byte("https://example.com/evil.pdf"))
		_, _ = w.Write(b)
	})
	_, _, e := c.Detail(context.Background(), 1)
	var f *Failure
	if !errors.As(e, &f) || f.Cause != "invalid_payload" {
		t.Fatalf("%v", e)
	}
}
func TestClient_403PausesEvenWithExhaustedBudget(t *testing.T) {
	g := sourcegate.NewMemory()
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403); _, _ = io.WriteString(w, "denied") }, WithGate(g), WithDownloadBudget(0))
	ctx := sourcegate.WithRunID(context.Background(), 22)
	_, _, e := c.Detail(ctx, 1)
	if !errors.Is(e, sourcegate.ErrSourcePaused) {
		t.Fatal(e)
	}
}

type timeoutBody struct{ sent bool }

func (r *timeoutBody) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, "%PDF-abc"), nil
	}
	return 0, timeoutErr{}
}
func (*timeoutBody) Close() error { return nil }

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
func TestDownload_PartialTimeoutDoesNotRetryIntoWriter(t *testing.T) {
	var n int
	tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
		n++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/pdf"}}, Body: &timeoutBody{}, ContentLength: -1}, nil
	})
	c := New(WithBaseURLs("http://127.0.0.1:1234", "http://127.0.0.1:1234"), WithTransport(tr), WithBackoffs(0), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1, Pages: 100, HTMLBytes: 5000000, FileBytes: 50000000, Redirects: 3}))
	var dst bytes.Buffer
	_, e := c.Download(context.Background(), "http://127.0.0.1:1234/a", &dst, 100)
	if e == nil || n != 1 || dst.String() != "%PDF-abc" {
		t.Fatalf("requests=%d bytes=%q err=%v", n, dst.String(), e)
	}
}
func TestClient_ExactBudgetAccepted(t *testing.T) {
	c, base := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "6")
		_, _ = io.WriteString(w, "%PDF-x")
	}, WithDownloadBudget(6))
	var dst bytes.Buffer
	_, e := c.Download(context.Background(), base+"/a", &dst, 20)
	if e != nil || dst.String() != "%PDF-x" {
		t.Fatalf("bytes=%q err=%v", dst.String(), e)
	}
}

func TestClient_BudgetStopsRetriesImmediately(t *testing.T) {
	var n atomic.Int32
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "12345678")
	}, WithBackoffs(0, 0, 0), WithDownloadBudget(3))
	_, _, e := c.Search(context.Background(), "x")
	if !errors.Is(e, ErrBudget) || n.Load() != 1 {
		t.Fatalf("requests=%d err=%v", n.Load(), e)
	}
}
func TestClient_SearchWaitingCanBeCanceled(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		_, _ = w.Write(fixture(t, "home.html"))
	})
	done := make(chan struct{})
	go func() { _, _, _ = c.Search(context.Background(), "first"); close(done) }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	returned := make(chan error, 1)
	go func() { _, _, e := c.Search(ctx, "second"); returned <- e }()
	select {
	case e := <-returned:
		if !errors.Is(e, context.Canceled) {
			t.Error(e)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("canceled search waited for session")
	}
	close(release)
	<-done
}

type blockingGate struct {
	gate    *sourcegate.Memory
	started chan struct{}
	release chan struct{}
}

func (g *blockingGate) Admit(ctx context.Context, id int64, s string) error {
	return g.gate.Admit(ctx, id, s)
}
func (g *blockingGate) Pause(ctx context.Context, id int64, s, reason, ref string) error {
	close(g.started)
	<-g.release
	return g.gate.Pause(ctx, id, s, reason, ref)
}
func (g *blockingGate) Clear(ctx context.Context, id int64, s, note string) error {
	return g.gate.Clear(ctx, id, s, note)
}

func TestClient_ChallengePausePrecedesNextAdmission(t *testing.T) {
	g := &blockingGate{sourcegate.NewMemory(), make(chan struct{}), make(chan struct{})}
	var n atomic.Int32
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) > 1 {
			w.WriteHeader(404)
			return
		}
		_, _ = io.WriteString(w, "<html>captcha</html>")
	}, WithGate(g))
	ctx := sourcegate.WithRunID(context.Background(), 9)
	first := make(chan error, 1)
	go func() { _, _, e := c.Detail(ctx, 1); first <- e }()
	<-g.started
	second := make(chan error, 1)
	go func() { _, _, e := c.Detail(ctx, 2); second <- e }()
	var early error
	var wasEarly bool
	select {
	case early = <-second:
		wasEarly = true
		t.Error("second request passed pending pause")
	case <-time.After(30 * time.Millisecond):
	}
	close(g.release)
	if e := <-first; !errors.Is(e, sourcegate.ErrSourcePaused) {
		t.Error(e)
	}
	if !wasEarly {
		early = <-second
	}
	if !errors.Is(early, sourcegate.ErrSourcePaused) {
		t.Error(early)
	}
	if n.Load() != 1 {
		t.Fatalf("requests=%d", n.Load())
	}
}

func TestClient_RetryAfter503Uses5xxCause(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Retry-After", "601"); w.WriteHeader(503) }, WithBackoffs(0))
	_, _, e := c.Detail(context.Background(), 1)
	var f *Failure
	if !errors.As(e, &f) || f.Cause != "http_5xx" || !errors.Is(e, ErrDeferred) {
		t.Fatalf("%v", e)
	}
}
func TestClient_OversizeErrorBodyStopsRetry(t *testing.T) {
	var n atomic.Int32
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(500)
		_, _ = io.WriteString(w, strings.Repeat("x", 30))
	}, WithBackoffs(0, 0), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1, Pages: 100, HTMLBytes: 10, FileBytes: 100, Redirects: 3}))
	_, _, e := c.Detail(context.Background(), 1)
	if !errors.Is(e, ErrTooLarge) || n.Load() != 1 {
		t.Fatalf("requests=%d err=%v", n.Load(), e)
	}
}

func TestClient_HugeRetryAfterDefers(t *testing.T) {
	var n atomic.Int32
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Retry-After", "999999999999999999999999999999")
		w.WriteHeader(429)
	}, WithBackoffs(0))
	_, _, e := c.Detail(context.Background(), 1)
	if !errors.Is(e, ErrDeferred) || n.Load() != 1 {
		t.Fatalf("requests=%d err=%v", n.Load(), e)
	}
}

func TestClient_EncodedTokenRedacted(t *testing.T) {
	const wire = "SENSITIVE&#45;TOKEN"
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		name := "home.html"
		if r.Method == http.MethodPost {
			name = "search-empty.html"
		}
		b := bytes.ReplaceAll(fixture(t, name), []byte("REDACTED-__VIEWSTATE"), []byte(wire))
		_, _ = w.Write(b)
	})
	_, b, e := c.Search(context.Background(), "x")
	if e != nil || len(b) != 1 || bytes.Contains(b[0], []byte(wire)) {
		t.Fatalf("encoded token retained or request failed: %v", e)
	}
}

func TestDownload_TimeoutAfterLimitDoesNotRetry(t *testing.T) {
	var n int
	tr := roundTripFunc(func(*http.Request) (*http.Response, error) {
		n++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/pdf"}}, Body: &timeoutBody{}, ContentLength: -1}, nil
	})
	c := New(WithBaseURLs("http://127.0.0.1:1234", "http://127.0.0.1:1234"), WithTransport(tr), WithBackoffs(0), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1, Pages: 100, HTMLBytes: 5000000, FileBytes: 50000000, Redirects: 3}))
	var dst bytes.Buffer
	_, e := c.Download(context.Background(), "http://127.0.0.1:1234/a", &dst, 8)
	if e == nil || n != 1 || dst.String() != "%PDF-abc" {
		t.Fatalf("requests=%d bytes=%q err=%v", n, dst.String(), e)
	}
}

func TestClient_TransportErrorDoesNotExposeSecrets(t *testing.T) {
	const token = "sensitive-token-in-transport"
	tr := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New(token) })
	c := New(WithBaseURLs("http://127.0.0.1:1234", "http://127.0.0.1:1234"), WithTransport(tr), WithBackoffs())
	_, _, e := c.Detail(context.Background(), 1)
	if e == nil {
		t.Fatal("missing error")
	}
	for e != nil {
		if strings.Contains(e.Error(), token) {
			t.Fatal("transport error leaked")
		}
		e = errors.Unwrap(e)
	}
}

func TestDetail_MissingInsideEmptyContainer(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<form></form><div id="ctrl_190596_91_Content">Không tìm thấy văn bản này</div>`)
	})
	_, _, e := c.Detail(context.Background(), 1)
	if !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
}

func TestDetail_TestFileHostAccepted(t *testing.T) {
	var base string
	c, url := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		b := bytes.ReplaceAll(fixture(t, "detail-218029.html"), []byte("https://datafiles.chinhphu.vn"), []byte(base))
		_, _ = w.Write(b)
	})
	base = url
	d, _, e := c.Detail(context.Background(), 1)
	if e != nil || len(d.Attachments) != 1 || !strings.HasPrefix(d.Attachments[0].URL, base) {
		t.Fatalf("detail=%+v err=%v", d, e)
	}
}

func TestClient_PageDeadlineIncludesResolution(t *testing.T) {
	r := func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 100*time.Millisecond {
			t.Error("resolution lacks request deadline")
			return nil, context.DeadlineExceeded
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	c := New(WithResolver(r), WithLimits(Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1, Pages: 100, HTMLBytes: 5000000, FileBytes: 50000000, Redirects: 3, PageTimeout: 10 * time.Millisecond}), WithBackoffs())
	_, _, e := c.Detail(context.Background(), 1)
	var f *Failure
	if !errors.As(e, &f) || f.Cause != "timeout" {
		t.Fatalf("%v", e)
	}
}

func TestSearch_RetainsShownTitleAndDate(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = w.Write(fixture(t, "home.html"))
			return
		}
		b := fixture(t, "search-number-142-2026-ND-CP.html")
		b = bytes.Replace(b, []byte("30/04/2026</span>"), []byte("30/04/2026 </span>"), 2)
		b = bytes.Replace(b, []byte("Luật Trí tuệ nhân tạo</span>"), []byte("Luật Trí tuệ nhân tạo </span>"), 1)
		_, _ = w.Write(b)
	})
	r, _, e := c.Search(context.Background(), "142")
	if e != nil || len(r.Items) != 1 || r.Items[0].Title == nil || !strings.HasSuffix(*r.Items[0].Title, " ") || r.Items[0].IssueDate == nil || !strings.HasSuffix(*r.Items[0].IssueDate, " ") {
		t.Fatalf("items=%+v err=%v", r.Items, e)
	}
}

func TestDetail_AttachmentWithoutClass(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		b := bytes.Replace(fixture(t, "detail-218029.html"), []byte(` class="view-file"`), nil, 1)
		_, _ = w.Write(b)
	})
	d, _, e := c.Detail(context.Background(), 1)
	if e != nil || len(d.Attachments) != 1 {
		t.Fatalf("attachments=%v err=%v", d.Attachments, e)
	}
}

func TestClient_RejectsIPv6TranslationAddress(t *testing.T) {
	c := New(WithResolver(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("64:ff9b::7f00:1")}}, nil
	}), WithTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("attempted transport for translation address")
		return nil, errors.New("blocked")
	})))
	_, e := c.Download(context.Background(), "https://datafiles.chinhphu.vn/a", io.Discard, 100)
	var f *Failure
	if !errors.As(e, &f) || f.Cause != "invalid_payload" {
		t.Fatal(e)
	}
}

func TestDetail_RepeatedFieldsPreserveOccurrences(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<div id="ctrl_190596_91_Content"><table><tr><td>Cơ quan ban hành</td><td> Chính phủ </td></tr><tr><td>Số ký hiệu</td><td>123</td></tr><tr><td>Cơ quan ban hành</td><td>Quốc hội</td></tr></table></div>`)
	})
	d, _, err := c.Detail(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Occurrences) != 3 || d.Occurrences[0].Value != " Chính phủ " || d.Occurrences[2].Value != "Quốc hội" || d.Issuer != "" {
		t.Fatalf("detail = %+v", d)
	}
}

func TestDetail_StructurallyAmbiguousRepeatedFieldIsExposed(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<div id="ctrl_190596_91_Content"><table><tr><td>Cơ quan ban hành</td><td>Chính phủ</td></tr><tr><td>Cơ quan ban hành</td><td>Quốc hội</td><td>khác</td></tr></table></div>`)
	})
	d, _, err := c.Detail(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Occurrences) != 2 || d.Issuer != "" {
		t.Fatal("ambiguous field was hidden")
	}
}

func TestDetail_AttachmentLabelsLossless(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<div id="ctrl_190596_91_Content"><table><tr><td>Tài liệu đính kèm</td><td><a href="https://datafiles.chinhphu.vn/docs/original.pdf">  Display
 label  </a></td></tr></table></div>`)
	})
	d, _, err := c.Detail(context.Background(), 1)
	if err != nil || len(d.Attachments) != 1 {
		t.Fatalf("detail = %+v, err = %v", d, err)
	}
	if got := d.Attachments[0]; got.Name != "original.pdf" || got.Label != `  Display
 label  ` || got.URL != "https://datafiles.chinhphu.vn/docs/original.pdf" {
		t.Fatalf("attachment = %+v", got)
	}
}

func TestDetail_CanonicalURLExcludesTracking(t *testing.T) {
	c, base := clientFor(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(fixture(t, "detail-218029.html")) })
	c.page = base + "/?tracking=discard"
	d, _, err := c.Detail(context.Background(), 218029)
	if err != nil {
		t.Fatal(err)
	}
	if d.URL != base+"/?docid=218029&pageid=27160" {
		t.Fatalf("URL %q", d.URL)
	}
}
