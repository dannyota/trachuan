package mcpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/limits"
	"danny.vn/trachuan/apps/server/internal/reports"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type reportSubmitter struct {
	account int64
	input   reports.Input
	calls   int
	err     error
}

func (s *reportSubmitter) Submit(_ context.Context, account int64, in reports.Input) (reports.Result, error) {
	s.account, s.input = account, in
	s.calls++
	return reports.Result{ReportID: 42, State: "open", Deduplicated: s.calls > 1}, s.err
}

func reportTool(t *testing.T, h *harness) *mcp.Tool {
	t.Helper()
	list, err := h.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		if tool.Name == "report_issue" {
			return tool
		}
	}
	t.Fatal("report_issue missing from tools/list")
	return nil
}

func reportArgs() map[string]any {
	return map[string]any{"country": "vn", "kind": "wrong_text", "handle": "vn:1#p11"}
}

func TestReportIssue_SchemaHasNoFreeText(t *testing.T) {
	h := newHarness(t, nil)
	tool := reportTool(t, h)
	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err = json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		kind, suggestion string
		valid            bool
	}{
		{"wrong_text", `{}`, true}, {"wrong_text", `{"note":"PRIVATE"}`, false},
		{"wrong_text", `{"document_number":"196-HDBT"}`, false},
		{"wrong_validity", `{"status_class":"partial","eff_from":"2026-01-01"}`, true},
		{"wrong_validity", `{"status_class":"PRIVATE"}`, false},
		{"wrong_validity", `{"issue_date":"2026-01-01"}`, false},
		{"broken_citation", `{"document_number":"196-HDBT"}`, true},
		{"broken_citation", `{"eff_to":"2026-01-01"}`, false},
		{"missing_document", `{"document_number":"1/2025/TT","issue_date":"2025-01-01"}`, true},
		{"missing_document", `{}`, false}, {"PRIVATE", `{}`, false},
	} {
		args := reportArgs()
		args["kind"] = tc.kind
		var suggestion any
		if err := json.Unmarshal([]byte(tc.suggestion), &suggestion); err != nil {
			t.Fatal(err)
		}
		args["suggestion"] = suggestion
		if got := resolved.Validate(args) == nil; got != tc.valid {
			t.Errorf("%s %s valid=%v", tc.kind, tc.suggestion, got)
		}
	}
	for _, member := range []string{"note", "source_url", "question"} {
		args := reportArgs()
		args[member] = "PRIVATE"
		if resolved.Validate(args) == nil {
			t.Errorf("schema accepts %s", member)
		}
	}
}

func TestReportIssue_Annotations(t *testing.T) {
	a := reportTool(t, newHarness(t, nil)).Annotations
	if a == nil || a.ReadOnlyHint || !a.IdempotentHint || a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
		t.Fatalf("annotations = %+v", a)
	}
}

func TestReportIssue_ResultShapeAndDedupe(t *testing.T) {
	s := &reportSubmitter{}
	h := newHarness(t, func(c *Config) { c.Reports = s })
	for _, deduplicated := range []bool{false, true} {
		r := callTool(t, h, "report_issue", reportArgs())
		out := resultJSON(t, r)
		if r.IsError || len(out) != 3 || out["report_id"] != float64(42) || out["state"] != "open" || out["deduplicated"] != deduplicated {
			t.Fatal(out)
		}
	}
	if s.account != 7 || s.input.Handle != "vn:1#p11" || s.input.Kind != "wrong_text" {
		t.Fatalf("submit = %+v", s)
	}
}

func TestReportIssue_ErrorsAndLimits(t *testing.T) {
	for _, code := range []reports.Error{reports.ErrInvalidRequest, reports.ErrReleaseExpired, reports.ErrRateLimited, reports.ErrUnavailable, reports.ErrUnauthenticated} {
		s := &reportSubmitter{err: code}
		h := newHarness(t, func(c *Config) { c.Reports = s })
		wantError(t, callTool(t, h, "report_issue", reportArgs()), string(code))
	}
	s := &reportSubmitter{err: errors.New("PRIVATE")}
	h := newHarness(t, func(c *Config) { c.Reports = s })
	wantError(t, callTool(t, h, "report_issue", reportArgs()), "unavailable")
	for _, raw := range []string{
		`{"country":"vn","kind":"wrong_text","handle":"vn:1","note":"PRIVATE"}`,
		`{"country":"vn","kind":"wrong_text","kind":"wrong_validity","handle":"vn:1"}`,
		`{"country":"vn","kind":"wrong_validity","handle":"vn:1","suggestion":{"note":"PRIVATE"}}`,
		`{"country":"vn","kind":"wrong_text","handle":null}`,
	} {
		wantError(t, callTool(t, h, "report_issue", json.RawMessage(raw)), "invalid_request")
	}
	if s.calls != 1 {
		t.Fatal("invalid fields reached service")
	}
	if strings.Contains(h.logs.String(), "PRIVATE") || strings.Contains(h.logs.String(), "vn:1") {
		t.Fatal("report input leaked to logs")
	}
	d := &denyingLimiter{}
	h = newHarness(t, func(c *Config) { c.Reports = s; c.Limiter = d })
	wantError(t, callTool(t, h, "report_issue", reportArgs()), "rate_limited")
	if d.class != limits.Read || s.calls != 1 {
		t.Fatal("report bypassed read bucket")
	}
	wantError(t, callTool(t, newHarness(t, nil), "report_issue", reportArgs()), "unavailable")
}

func TestGuide_MentionsReporting(t *testing.T) {
	out := resultJSON(t, callTool(t, newHarness(t, nil), "guide", map[string]any{"country": "vn"}))
	guide := out["guide"].(string)
	for _, text := range []string{"report_issue", "wrong text", "validity", "citation", "missing document", "structured fields"} {
		if !strings.Contains(guide, text) {
			t.Errorf("guide missing %q", text)
		}
	}
}
