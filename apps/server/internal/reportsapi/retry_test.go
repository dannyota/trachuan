package reportsapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/apps/server/internal/websession"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const retryKey = "vn:vbpl:03206030-6f7b-11f1-95c5-bd4e620e4897"

type retryCorpus struct{}

func (retryCorpus) Document(context.Context, string) (reports.Document, bool, error) {
	return reports.Document{Number: "1/2024/TT"}, true, nil
}
func (retryCorpus) AnchorExists(context.Context, string, string) (bool, error) { return true, nil }
func (retryCorpus) HasNumber(context.Context, string) (bool, error)            { return true, nil }
func (retryCorpus) HasNumberSegment(context.Context, string) (bool, error)     { return true, nil }

type retryResolver struct{}

func (retryResolver) Resolve(context.Context, string, string) (reports.Lease, error) {
	return reports.Lease{Release: "20990101", Corpus: retryCorpus{}}, nil
}

type retryTx struct {
	counts   reports.AccountCounts
	retained int64
	oldest   time.Time
}

func (x *retryTx) Begin(context.Context) (reports.Transaction, error) { return x, nil }
func (*retryTx) Commit(context.Context) error                         { return nil }
func (*retryTx) Rollback(context.Context) error                       { return nil }
func (*retryTx) LockAccountForReport(context.Context, reports.LockAccountForReportParams) (reports.AccountState, error) {
	return reports.AccountState{Exists: true}, nil
}
func (*retryTx) LockReports(context.Context) error { return nil }
func (x *retryTx) CountAccountReports(context.Context, reports.CountAccountReportsParams) (reports.AccountCounts, error) {
	return x.counts, nil
}
func (x *retryTx) OldestAccountReport(context.Context, reports.CountAccountReportsParams) (time.Time, error) {
	return x.oldest, nil
}
func (x *retryTx) CountRetainedReports(context.Context) (int64, error) { return x.retained, nil }
func (*retryTx) FindOpenReport(context.Context, reports.FindOpenReportParams) (reports.Result, bool, error) {
	return reports.Result{}, false, nil
}
func (*retryTx) InsertReport(context.Context, reports.InsertReportParams) (reports.Result, error) {
	panic("unexpected insert")
}

func TestReports_AdmissionCapRetry(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		counts   reports.AccountCounts
		retained int64
		seconds  int
	}{
		{"daily", reports.AccountCounts{Daily: 20}, 0, 121},
		{"open", reports.AccountCounts{Open: 200}, 0, 3600},
		{"account retained", reports.AccountCounts{Retained: 1000}, 0, 3600},
		{"global retained", reports.AccountCounts{}, 100000, 3600},
		{"daily and open", reports.AccountCounts{Daily: 20, Open: 200}, 0, 3600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &retryTx{counts: tc.counts, retained: tc.retained, oldest: now.Add(-24*time.Hour + 120*time.Second + 500*time.Millisecond)}
			svc := &reports.Service{Store: tx, Resolver: retryResolver{}, Clock: func() time.Time { return now }}
			db := &testDB{now: now}
			m := &websession.Manager{Store: db, Clock: func() time.Time { return now }, PublicURL: "https://example.invalid"}
			h := m.Middleware(m.CSRF(New(Config{Store: db, Reports: svc, Limiter: limits.New(func() time.Time { return now })})))
			raw := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
			w := call(h, raw, "POST", "/api/v1/reports", `{"country":"vn","kind":"wrong_text","handle":"`+retryKey+`"}`)
			expect(t, w, 429)
			if got := w.Header().Get("Retry-After"); got != fmt.Sprint(tc.seconds) {
				t.Errorf("web retry %s, want %d", got, tc.seconds)
			}
			clock := func() time.Time { return now }
			v := &credential.Verifier{Store: bearerStore{}, Audience: "https://example.invalid/mcp", Clock: clock}
			endpoint := credential.Middleware(v, mcpapi.New(mcpapi.Config{Reports: svc, Limiter: limits.New(clock), Clock: clock}))
			client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
			session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: "https://example.invalid/mcp", HTTPClient: &http.Client{Transport: localTransport{endpoint}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = session.Close() }()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "report_issue", Arguments: map[string]any{"country": "vn", "kind": "wrong_text", "handle": retryKey}})
			if err != nil || !result.IsError {
				t.Fatalf("MCP result %v: %v", result, err)
			}
			encoded, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), fmt.Sprintf("Retry in %d seconds.", tc.seconds)) {
				t.Errorf("MCP response %s", encoded)
			}
		})
	}
}
