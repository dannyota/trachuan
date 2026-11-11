package reportsapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/apps/server/internal/store"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testDB struct {
	rows      []store.IssueReport
	now       time.Time
	dbErr     error
	submitErr error
	input     reports.Input
	account   int64
}

func (s *testDB) ListOwnReports(_ context.Context, p store.ListOwnReportsParams) ([]store.IssueReport, error) {
	if s.dbErr != nil {
		return nil, s.dbErr
	}
	var rows []store.IssueReport
	for _, r := range s.rows {
		if r.AccountID == p.AccountID && (p.CursorCreatedAt == nil || r.CreatedAt.Before(*p.CursorCreatedAt) || (r.CreatedAt.Equal(*p.CursorCreatedAt) && r.ID < *p.CursorID)) {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID > rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	if len(rows) > int(p.PageSize) {
		rows = rows[:p.PageSize]
	}
	return rows, nil
}
func (s *testDB) WithdrawOwnReport(_ context.Context, p store.WithdrawOwnReportParams) (int64, error) {
	if s.dbErr != nil {
		return 0, s.dbErr
	}
	for i := range s.rows {
		r := &s.rows[i]
		if r.ID == p.ID && r.AccountID == p.AccountID && r.State == "open" {
			r.State = "withdrawn"
			return 1, nil
		}
	}
	return 0, nil
}
func (s *testDB) GetSession(context.Context, string) (store.GetSessionRow, error) {
	return store.GetSessionRow{ID: 1, AccountID: 1, CsrfSecret: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", 32))), CreatedAt: s.now, LastSeenAt: s.now, ExpiresAt: s.now.Add(time.Hour)}, nil
}
func (*testDB) CreateSession(context.Context, store.CreateSessionParams) (store.CreateSessionRow, error) {
	panic("unused")
}
func (*testDB) TouchSession(context.Context, string) (int64, error)  { panic("unused") }
func (*testDB) RevokeSession(context.Context, string) (int64, error) { panic("unused") }
func (*testDB) RevokeOwnSession(context.Context, store.RevokeOwnSessionParams) (int64, error) {
	panic("unused")
}

type testSubmit struct{ db *testDB }

func (s testSubmit) Submit(_ context.Context, account int64, in reports.Input) (reports.Result, error) {
	s.db.input = in
	s.db.account = account
	if s.db.submitErr != nil {
		return reports.Result{}, s.db.submitErr
	}
	if account != 1 || in.Kind != "wrong_text" {
		return reports.Result{}, reports.ErrInvalidRequest
	}
	return reports.Result{ReportID: 9, State: "open", Deduplicated: true}, nil
}
func setup() (http.Handler, *testDB, string) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	db := &testDB{now: now}
	m := &websession.Manager{Store: db, Clock: func() time.Time { return now }, PublicURL: "https://example.invalid"}
	raw := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	return m.Middleware(m.CSRF(New(Config{Store: db, Reports: testSubmit{db}, Limiter: limits.New(func() time.Time { return now })}))), db, raw
}
func call(h http.Handler, raw, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://example.invalid"+path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
	r.Header.Set("Origin", "https://example.invalid")
	r.Header.Set("X-CSRF-Token", base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", 32))))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
	}
}
func row(id, account int64, now time.Time) store.IssueReport {
	return store.IssueReport{ID: id, AccountID: account, Kind: "wrong_text", DocumentKey: "vn:1", Suggestion: json.RawMessage(`{}`), State: "open", CreatedAt: now, ChangedAt: now}
}

type bearerStore struct{}

func (bearerStore) GetCredential(context.Context, string) (store.GetCredentialRow, error) {
	return store.GetCredentialRow{AccountID: 1, ExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
func (bearerStore) TouchPersonalToken(context.Context, string) error { return nil }

type localTransport struct{ handler http.Handler }

func (l localTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer trc_"+strings.Repeat("A", 43))
	w := httptest.NewRecorder()
	l.handler.ServeHTTP(w, r)
	return w.Result(), nil
}
func TestReports_SubmitEqualsMCP(t *testing.T) {
	h, db, raw := setup()
	body := `{"country":"vn","kind":"wrong_text","handle":"vn:1"}`
	w := call(h, raw, "POST", "/api/v1/reports", body)
	expect(t, w, 200)
	webInput, webAccount := db.input, db.account
	var webResult reports.Result
	if err := json.Unmarshal(w.Body.Bytes(), &webResult); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return db.now }
	verifier := &credential.Verifier{Store: bearerStore{}, Audience: "https://example.invalid/mcp", Clock: clock}
	endpoint := credential.Middleware(verifier, mcpapi.New(mcpapi.Config{Reports: testSubmit{db}, Limiter: limits.New(clock), Clock: clock}))
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: "https://example.invalid/mcp", HTTPClient: &http.Client{Transport: localTransport{endpoint}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "report_issue", Arguments: map[string]any{"country": "vn", "kind": "wrong_text", "handle": "vn:1"}})
	if err != nil || result.IsError {
		t.Fatalf("MCP result %v: %v", result, err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var mcpResult reports.Result
	if err = json.Unmarshal(encoded, &mcpResult); err != nil {
		t.Fatal(err)
	}
	if webResult != mcpResult || webAccount != db.account || !reflect.DeepEqual(webInput, db.input) {
		t.Fatalf("web %v / MCP %v, inputs %v / %v", webResult, mcpResult, webInput, db.input)
	}
}
func TestReports_OwnOnly(t *testing.T) {
	h, db, raw := setup()
	db.rows = []store.IssueReport{row(1, 1, db.now), row(2, 2, db.now)}
	w := call(h, raw, "GET", "/api/v1/me/reports", "")
	expect(t, w, 200)
	if strings.Contains(w.Body.String(), "account_id") || strings.Contains(w.Body.String(), `"report_id":2`) || !strings.Contains(w.Body.String(), `"report_id":1`) {
		t.Fatal(w.Body.String())
	}
	expect(t, call(h, raw, "DELETE", "/api/v1/me/reports/2", ""), 404)
}
func TestReports_WithdrawOpenOnly(t *testing.T) {
	h, db, raw := setup()
	db.rows = []store.IssueReport{row(1, 1, db.now), row(2, 1, db.now)}
	db.rows[1].State = "accepted"
	expect(t, call(h, raw, "DELETE", "/api/v1/me/reports/2", ""), 404)
	expect(t, call(h, raw, "DELETE", "/api/v1/me/reports/1", ""), 204)
	expect(t, call(h, raw, "DELETE", "/api/v1/me/reports/1", ""), 404)
}
func TestReports_OwnListPagination(t *testing.T) {
	h, db, raw := setup()
	for i := int64(1); i <= 101; i++ {
		db.rows = append(db.rows, row(i, 1, db.now))
	}
	w := call(h, raw, "GET", "/api/v1/me/reports", "")
	expect(t, w, 200)
	var page struct {
		Reports []map[string]any `json:"reports"`
		Next    string           `json:"next"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Reports) != 100 || page.Next == "" {
		t.Fatalf("page %s: %v", w.Body.String(), err)
	}
	w = call(h, raw, "GET", "/api/v1/me/reports?next="+page.Next, "")
	expect(t, w, 200)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Reports) != 1 || page.Reports[0]["report_id"] != float64(1) || strings.Contains(w.Body.String(), `"next"`) {
		t.Fatalf("page %s: %v", w.Body.String(), err)
	}
	expect(t, call(h, raw, "DELETE", fmt.Sprintf("/api/v1/me/reports/%d", 1), ""), 204)
	expect(t, call(h, raw, "GET", "/api/v1/me/reports?next=garbage", ""), 400)
}
func TestReports_Contract(t *testing.T) {
	c, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	cov := contracttest.NewCoverage(c, []string{"/api/v1/reports", "/api/v1/me/reports", "/api/v1/me/reports/{id}"})
	for _, method := range []string{"POST", "GET", "DELETE"} {
		path, actual, body := "/api/v1/me/reports", "/api/v1/me/reports", ""
		scenarios := []string{"success", "unauthenticated", "limit", "unavailable", "invalid"}
		if method == "POST" {
			path, actual, body = "/api/v1/reports", "/api/v1/reports", `{"country":"vn","kind":"wrong_text","handle":"vn:1"}`
			scenarios = append(scenarios, "csrf", "expired", "large")
		}
		if method == "DELETE" {
			path, actual = "/api/v1/me/reports/{id}", "/api/v1/me/reports/1"
			scenarios = []string{"success", "unauthenticated", "limit", "unavailable", "csrf", "missing"}
		}
		for _, scenario := range scenarios {
			t.Run(method+scenario, func(t *testing.T) {
				h, db, raw := setup()
				db.rows = []store.IssueReport{row(1, 1, db.now)}
				target, payload, token := actual, body, raw
				want := 200
				if method == "DELETE" {
					want = 204
				}
				switch scenario {
				case "unauthenticated":
					token = "invalid"
					want = 401
				case "limit":
					for range 120 {
						expect(t, call(h, raw, "GET", "/api/v1/me/reports", ""), 200)
					}
					want = 429
				case "unavailable":
					db.dbErr = errors.New("private")
					db.submitErr = reports.ErrUnavailable
					want = 503
				case "invalid":
					if method == "POST" {
						payload = `{}`
					} else {
						target += "?next=bad"
					}
					want = 400
				case "expired":
					db.submitErr = reports.ErrReleaseExpired
					want = 409
				case "large":
					payload = strings.Repeat(" ", 64*1024+1)
					want = 413
				case "missing":
					target = "/api/v1/me/reports/2"
					want = 404
				case "csrf":
					r := httptest.NewRequest(method, "https://example.invalid"+target, strings.NewReader(payload))
					r.AddCookie(&http.Cookie{Name: "__Host-session", Value: raw})
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					expect(t, w, 403)
					cov.Check(t, method, path, w)
					return
				}
				w := call(h, token, method, target, payload)
				expect(t, w, want)
				cov.Check(t, method, path, w)
			})
		}
	}
	if missing := cov.Missing(); len(missing) > 0 {
		t.Fatal(missing)
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"country":"vn","kind":"wrong_text","handle":"vn:1"}`, true},
		{`{"country":"vn","kind":"wrong_text","handle":"vn:1","suggestion":{"issue_date":"2024-01-01"}}`, false},
		{`{"country":"vn","kind":"missing_document","suggestion":{"document_number":"1/2024/TT"}}`, true},
		{`{"country":"vn","kind":"missing_document","suggestion":{}}`, false},
	} {
		if err := c.ValidateRequest("POST", "/api/v1/reports", []byte(tc.body)); (err == nil) != tc.valid {
			t.Fatalf("request %s: %v", tc.body, err)
		}
	}
}
func (*testDB) GetAccount(context.Context, int64) (store.Account, error) { panic("unused") }

func TestReports_ClientSuggestionFields(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/schema/gen/openapi.ts")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "        ReportInput: {")
	end := strings.Index(text, "        ReportKind:")
	if start < 0 || end <= start {
		t.Fatal("generated ReportInput missing")
	}
	input := text[start:end]
	for _, field := range []string{"status_class?:", "eff_from?:", "eff_to?:", "document_number?:", "issue_date?:"} {
		if !strings.Contains(input, field) {
			t.Errorf("generated ReportInput lacks %s", field)
		}
	}
}

func TestReports_MalformedCursor(t *testing.T) {
	h, _, raw := setup()
	for _, query := range []string{"?next=", "?next=%", "?next=abc&next=abc", "?next=" + strings.Repeat("A", 10000)} {
		t.Run(query[:min(len(query), 30)], func(t *testing.T) { expect(t, call(h, raw, "GET", "/api/v1/me/reports"+query, ""), 400) })
	}
}
