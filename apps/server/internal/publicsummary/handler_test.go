package publicsummary

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
)

const fixturePayload = `{"number":"1","title":"Title","type":"law","issuer":"Issuer","issued":null,"has_text":true,"text_complete":false,"source_url":"http://source.test","relations":[{"kind":"cites","direction":"outgoing","number":"2"}],"relations_total":1,"nodes":[{"code":"1","name":"Topic"}],"outline":[{"kind":"chuong","label":"Chương I","anchor":"c1","heading":"Chapter"},{"kind":"dieu","label":"Điều 1","anchor":"a1","heading":"Article"}]}`

type fixturePool struct {
	memoryPool
	row    Row
	items  []Item
	rowErr error
}

func (p *fixturePool) Row(_ context.Context, key string) (Row, error) {
	if p.rowErr != nil {
		return Row{}, p.rowErr
	}
	if key != p.row.Key {
		return Row{}, ErrNotFound
	}
	return p.row, nil
}
func (p *fixturePool) List(context.Context) ([]Item, error) { return p.items, nil }

func handlerFixture(t *testing.T) (*Handler, *fixturePool, *Release, *atomic.Int64) {
	t.Helper()
	release := &Release{Database: "corpus_vn_20990101", ID: "20990101", Projection: 1}
	pool := &fixturePool{memoryPool: memoryPool{closed: make(chan struct{})}, row: Row{Key: "vn:vbpl:1", Indexed: true, StatusClass: "in_force", StatusBasis: "source_status", IsCurrent: true, Payload: []byte(fixturePayload), GapKinds: []string{"consolidated_status_unresolved"}}, items: []Item{{"vn:vbpl:1", true}}}
	calls := &atomic.Int64{}
	clock := func() time.Time { return time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC) }
	manager := NewManager(func(context.Context) (Release, error) { calls.Add(1); return *release, nil }, func(context.Context, string) (Pool, error) { return pool, nil }, clock)
	t.Cleanup(manager.Close)
	return NewHandler(manager, true, clock), pool, release, calls
}

func summaryCall(h http.Handler, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/public/documents", h)
	mux.Handle("GET /api/v1/public/documents/{key}", h)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

func TestSummary_ActualResponsesMatchClosedContract(t *testing.T) {
	h, _, release, _ := handlerFixture(t)
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverage(contract, []string{"/api/v1/public/documents/{key}", "/api/v1/public/documents"})
	for _, approved := range []bool{false, true} {
		release.HeadingsApproved = approved
		for _, path := range []string{"/api/v1/public/documents/vn:vbpl:1", "/api/v1/public/documents"} {
			w := summaryCall(h, path)
			if w.Code != 200 {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
			schemaPath := path
			if strings.Contains(path, "/vn:") {
				schemaPath = "/api/v1/public/documents/{key}"
			}
			coverage.Check(t, "GET", schemaPath, w)
			if w.Header().Get("ETag") != fmt.Sprintf(`"%x"`, sha256.Sum256(w.Body.Bytes())) || w.Header().Get("Cache-Control") != "public, max-age=1, must-revalidate" {
				t.Fatal("incorrect freshness headers", w.Header())
			}
			if schemaPath != "/api/v1/public/documents" {
				var data map[string]any
				if err = json.Unmarshal(w.Body.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				document := data["document"].(map[string]any)
				if document["source_url"] != nil || data["status_date"] != "2099-01-01" {
					t.Fatalf("document: %v", data)
				}
				for _, entry := range data["outline"].([]any) {
					_, has := entry.(map[string]any)["heading"]
					if has != approved {
						t.Fatalf("mixed heading approval: %v", data)
					}
				}
			}
		}
	}
	metadata := h.manager.metadata
	for _, path := range []string{"/api/v1/public/documents/vn:vbpl:1", "/api/v1/public/documents"} {
		h.manager.metadata = metadata
		schemaPath := path
		if strings.Contains(path, "/vn:") {
			schemaPath = "/api/v1/public/documents/{key}"
		}
		coverage.Check(t, "GET", schemaPath, summaryCall(h, path+"?bad=1"))
		h.enabled = false
		coverage.Check(t, "GET", schemaPath, summaryCall(h, path))
		h.enabled = true
		h.manager.metadata = func(context.Context) (Release, error) { return Release{}, ErrUnavailable }
		coverage.Check(t, "GET", schemaPath, summaryCall(h, path))
	}
	if missing := coverage.Missing(); len(missing) != 0 {
		t.Fatalf("contract coverage: %v", missing)
	}
}

func TestSummary_ErrorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, path  string
		enabled     bool
		projection  int
		unavailable bool
		code        int
		errorCode   string
	}{
		{"invalid before unavailable", "/api/v1/public/documents/bad?x=1", false, 0, true, 400, "invalid_request"},
		{"invalid key", "/api/v1/public/documents/vn:%20", true, 1, false, 400, "invalid_request"},
		{"anchor", "/api/v1/public/documents/vn:1%23a", true, 1, false, 400, "invalid_request"},
		{"list query", "/api/v1/public/documents?x", false, 0, true, 400, "invalid_request"},
		{"unavailable before flag", "/api/v1/public/documents/vn:vbpl:1", false, 0, true, 503, "unavailable"},
		{"flag off", "/api/v1/public/documents/vn:vbpl:1", false, 1, false, 404, "not_found"},
		{"capability absent", "/api/v1/public/documents", true, 0, false, 404, "not_found"},
		{"missing key", "/api/v1/public/documents/vn:vbpl:2", true, 1, false, 404, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, release, _ := handlerFixture(t)
			h.enabled = tc.enabled
			release.Projection = tc.projection
			if tc.unavailable {
				h.manager.metadata = func(context.Context) (Release, error) { return Release{}, ErrUnavailable }
			}
			w := summaryCall(h, tc.path)
			if w.Code != tc.code || !strings.Contains(w.Body.String(), `"code":"`+tc.errorCode+`"`) {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

type heldWriter struct {
	*httptest.ResponseRecorder
	entered chan<- struct{}
	proceed <-chan struct{}
}

func (w heldWriter) Write(raw []byte) (int, error) {
	w.entered <- struct{}{}
	<-w.proceed
	return w.ResponseRecorder.Write(raw)
}

func TestSummary_AdmissionIncludesWarmRowsThroughWrite(t *testing.T) {
	h, _, _, calls := handlerFixture(t)
	if w := summaryCall(h, "/api/v1/public/documents/vn:vbpl:1"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	entered := make(chan struct{}, 8)
	proceed := make(chan struct{})
	done := make(chan struct{}, 8)
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/public/documents/{key}", h)
	for range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			mux.ServeHTTP(heldWriter{httptest.NewRecorder(), entered, proceed}, httptest.NewRequest("GET", "/api/v1/public/documents/vn:vbpl:1", nil))
		}()
	}
	for range 8 {
		<-entered
	}
	originalMetadata := h.manager.metadata
	h.enabled = false
	h.manager.metadata = func(context.Context) (Release, error) { calls.Add(1); return Release{}, ErrUnavailable }
	before := calls.Load()
	for _, path := range []string{"/api/v1/public/documents/vn:vbpl:1", "/api/v1/public/documents"} {
		w := summaryCall(h, path)
		if w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"busy"`) || w.Header().Get("Retry-After") != "5" {
			t.Fatalf("admission: %d %s", w.Code, w.Body.String())
		}
	}
	if calls.Load() != before {
		t.Fatal("full admission resolved release")
	}
	if w := summaryCall(h, "/api/v1/public/documents?bad=1"); w.Code != 400 {
		t.Fatal("validation lost precedence", w.Code)
	}
	h.manager.metadata = originalMetadata
	h.enabled = true
	close(proceed)
	for range 8 {
		<-done
	}
	if w := summaryCall(h, "/api/v1/public/documents"); w.Code != 200 {
		t.Fatalf("slot leaked: %d", w.Code)
	}
}

func TestSummary_ListLimits(t *testing.T) {
	for _, tc := range []struct {
		name        string
		count, size int
		want        int
	}{
		{"ten thousand", 10000, 4, 200}, {"too many", 10001, 4, 503}, {"encoded byte cap", 1000, 1100, 503}, {"encoded escapes count", 1000, 200, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, p, _, _ := handlerFixture(t)
			p.items = make([]Item, tc.count)
			for i := range p.items {
				key := strings.Repeat("x", tc.size)
				if tc.name == "encoded escapes count" {
					key = strings.Repeat("<", tc.size)
				}
				p.items[i] = Item{Key: "vn:" + key, Index: true}
			}
			w := summaryCall(h, "/api/v1/public/documents")
			if w.Code != tc.want {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
			if tc.want == 503 && !strings.Contains(w.Body.String(), `"code":"too_large"`) {
				t.Fatal(w.Body.String())
			}
		})
	}
}

func TestSummary_StatusChangesOnWarmRowAtVietnamMidnight(t *testing.T) {
	h, p, _, _ := handlerFixture(t)
	from := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	p.row.EffectiveFrom, p.row.EffectiveTo = &from, &to
	now := time.Date(2099, 1, 1, 16, 59, 59, 500000000, time.UTC)
	h.clock = func() time.Time { return now }
	for _, tc := range []struct {
		date    string
		current bool
		maxAge  string
	}{
		{"2099-01-01", true, "public, max-age=0, must-revalidate"},
		{"2099-01-02", false, "public, max-age=300, must-revalidate"},
	} {
		w := summaryCall(h, "/api/v1/public/documents/vn:vbpl:1")
		var body struct {
			StatusDate string `json:"status_date"`
			Document   struct {
				Status struct {
					Current bool `json:"current"`
				} `json:"status"`
			} `json:"document"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.StatusDate != tc.date || body.Document.Status.Current != tc.current || w.Header().Get("Cache-Control") != tc.maxAge {
			t.Fatalf("midnight: %d %s %s", w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
		}
		now = now.Add(time.Second)
	}
}

func TestSummary_PoolBusyAndAvailabilityTransitions(t *testing.T) {
	h, _, release, _ := handlerFixture(t)
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := contracttest.NewCoverage(contract, []string{"/api/v1/public/documents", "/api/v1/public/documents/{key}"})
	lease, err := h.manager.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease.entry.slot <- struct{}{}
	for _, path := range []string{"/api/v1/public/documents/vn:vbpl:1", "/api/v1/public/documents"} {
		w := summaryCall(h, path)
		if w.Code != 503 || w.Header().Get("Retry-After") != "5" || !strings.Contains(w.Body.String(), `"code":"busy"`) {
			t.Fatalf("pool full: %d %s", w.Code, w.Body.String())
		}
		schemaPath := path
		if path != "/api/v1/public/documents" {
			schemaPath = "/api/v1/public/documents/{key}"
		}
		coverage.Check(t, "GET", schemaPath, w)
	}
	<-lease.entry.slot
	lease.Done()
	for _, projection := range []int{1, 0, 1} {
		release.Projection = projection
		enabled, featureErr := h.Available(context.Background(), release.ID)
		if featureErr != nil || enabled != (projection == 1) {
			t.Fatalf("capability %d: %v %v", projection, enabled, featureErr)
		}
		w := summaryCall(h, "/api/v1/public/documents/vn:vbpl:1")
		want := 200
		if projection == 0 {
			want = 404
		}
		if w.Code != want {
			t.Fatalf("transition: %d %s", w.Code, w.Body.String())
		}
	}
	h.enabled = false
	if enabled, featureErr := h.Available(context.Background(), release.ID); featureErr != nil || enabled {
		t.Fatalf("flag off: %v %v", enabled, featureErr)
	}
	if _, err = h.Available(context.Background(), "20990102"); err == nil {
		t.Fatal("mixed status release accepted")
	}
	*release = Release{}
	if w := summaryCall(h, "/api/v1/public/documents"); w.Code != 503 {
		t.Fatalf("no active release with flag off: %d", w.Code)
	}
}

func TestSummary_ValidityAndHTTPSProjection(t *testing.T) {
	h, p, release, _ := handlerFixture(t)
	p.row.ValidityConflict = true
	p.row.Payload = []byte(strings.Replace(fixturePayload, "http://source.test", "https://source.test", 1))
	w := summaryCall(h, "/api/v1/public/documents/vn:vbpl:1")
	var body struct {
		Document document `json:"document"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Document.SourceURL == nil || *body.Document.SourceURL != "https://source.test" || !body.Document.Status.Unresolved || body.Document.Status.Current {
		t.Fatalf("projection: %s", w.Body.String())
	}
	if strings.Join(body.Document.GapKinds, ",") != "consolidated_status_unresolved,missing_effective_date" {
		t.Fatalf("gaps: %v", body.Document.GapKinds)
	}
	release.HeadingsApproved = true
	p.row.GapKinds = []string{"unknown_future_gap"}
	if w = summaryCall(h, "/api/v1/public/documents/vn:vbpl:1"); w.Code != 503 {
		t.Fatalf("unknown gap leaked: %d %s", w.Code, w.Body.String())
	}
}

type elapsedPool struct {
	*fixturePool
	finish func()
}

func (p elapsedPool) Row(ctx context.Context, key string) (Row, error) {
	p.finish()
	return p.fixturePool.Row(ctx, key)
}

func TestSummary_QueryCrossingMidnightCannotCacheYesterdaysStatus(t *testing.T) {
	h, p, _, _ := handlerFixture(t)
	now := time.Date(2099, 1, 1, 16, 59, 59, 0, time.UTC)
	h.clock = func() time.Time { return now }
	h.manager.open = func(context.Context, string) (Pool, error) {
		return elapsedPool{p, func() { now = now.Add(2 * time.Second) }}, nil
	}
	w := summaryCall(h, "/api/v1/public/documents/vn:vbpl:1")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatalf("cross-midnight cache lifetime: %d %s", w.Code, w.Header().Get("Cache-Control"))
	}
}
