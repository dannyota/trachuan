package reportsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
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

type paritySubmitter struct{}

func (paritySubmitter) Submit(context.Context, int64, reports.Input) (reports.Result, error) {
	return reports.Result{ReportID: 9, State: "open"}, nil
}
func TestReports_MissingDocumentHandleParity(t *testing.T) {
	_, db, raw := setup()
	clock := func() time.Time { return db.now }
	submit := paritySubmitter{}
	sessions := &websession.Manager{Store: db, Clock: clock, PublicURL: "https://example.invalid"}
	web := sessions.Middleware(sessions.CSRF(New(Config{Store: db, Reports: submit, Limiter: limits.New(clock)})))
	verifier := &credential.Verifier{Store: bearerStore{}, Audience: "https://example.invalid/mcp", Clock: clock}
	endpoint := credential.Middleware(verifier, mcpapi.New(mcpapi.Config{Reports: submit, Limiter: limits.New(clock), Clock: clock}))
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: "https://example.invalid/mcp", HTTPClient: &http.Client{Transport: localTransport{endpoint}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	for _, tc := range []struct {
		name, handle   string
		present, valid bool
	}{{"absent", "", false, true}, {"empty", "", true, false}, {"valid", "vn:1#dieu-1", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"country": "vn", "kind": "missing_document", "suggestion": map[string]any{"document_number": "1/2024/TT"}}
			if tc.present {
				args["handle"] = tc.handle
			}
			body, err := json.Marshal(args)
			if err != nil {
				t.Fatal(err)
			}
			webResult := call(web, raw, "POST", "/api/v1/reports", string(body))
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "report_issue", Arguments: args})
			if err != nil {
				t.Fatal(err)
			}
			if (webResult.Code == http.StatusOK) != tc.valid || result.IsError == tc.valid {
				t.Fatalf("web %d %s MCP %#v", webResult.Code, webResult.Body, result)
			}
			if tc.valid {
				var webValue, mcpValue reports.Result
				encoded, err := json.Marshal(result.StructuredContent)
				if err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(webResult.Body.Bytes(), &webValue); err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(encoded, &mcpValue); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(webValue, mcpValue) {
					t.Fatalf("different results: %+v %+v", webValue, mcpValue)
				}
			}
			if !tc.valid {
				if !strings.Contains(webResult.Body.String(), "invalid_request") {
					t.Fatal(webResult.Body.String())
				}
				encoded, _ := json.Marshal(result.StructuredContent)
				if !strings.Contains(string(encoded), "invalid_request") {
					t.Fatalf("MCP error %s", encoded)
				}
			}
		})
	}
}
