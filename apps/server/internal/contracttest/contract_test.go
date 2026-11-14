package contracttest

import (
	"net/http/httptest"
	"strings"

	"testing"
)

func TestRequestSchema_Constraints(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, body string
		valid      bool
	}{
		{"/api/v1/search", `{"country":"vn","query":"law","limit":1}`, true},
		{"/api/v1/search", `{"country":"us","query":"law"}`, false},
		{"/api/v1/search", `{"country":"vn","query":"law","limit":0}`, false},
		{"/api/v1/search", `{"country":"vn","query":"law","limit":51}`, false},
		{"/api/v1/search", `{"country":"vn","query":"law","limit":1.5}`, false},
		{"/api/v1/search", `{"country":"vn","query":"law","limit":50.0000000000000001}`, false},
		{"/api/v1/search", `{"country":"vn","query":"law","detail":"other"}`, false},
		{"/api/v1/search", `{"country":"vn","query":"law","extra":1}`, false},
		{"/api/v1/read", `{"country":"vn","items":[{},{},{},{},{},{},{},{},{}]}`, false},
		{"/api/v1/me/tokens", `{"name":"test","days":null}`, true},
		{"/api/v1/me/tokens", `{"name":"test","days":"bad"}`, false},
		{"/api/v1/me/tokens", `{"name":"test","days":366}`, false},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			err := c.ValidateRequest("POST", tc.path, []byte(tc.body))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestCoverage_ReportsMissing(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	coverage := NewCoverage(c, []string{"/api/v1/status"})
	if len(coverage.Missing()) == 0 {
		t.Fatal("unexercised statuses were not reported")
	}
}

func TestCoverage_RejectsUndocumentedAndInvalidResponses(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cov := NewCoverage(c, []string{"/api/v1/status"})
	w := httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.WriteString(`{"release":"r","documents":1.5,"provisions":1,"nodes":1}`)
	if err := cov.observe("GET", "/api/v1/status", w); err == nil {
		t.Fatal("fractional integer accepted")
	}
	w = httptest.NewRecorder()
	w.WriteHeader(418)
	if err := cov.observe("GET", "/api/v1/status", w); err == nil {
		t.Fatal("undocumented status accepted")
	}
	w = httptest.NewRecorder()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.WriteString(`{"release":"r","documents":1,"provisions":1,"nodes":1}`)
	before := len(cov.Missing())
	if err := cov.observe("GET", "/api/v1/status", w); err != nil {
		t.Fatal(err)
	}
	if len(cov.Missing()) != before-1 {
		t.Fatal("valid response was not counted")
	}
}

func TestSchema_DateFormats(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, schema, body string
		valid              bool
	}{
		{"valid date", "SearchInput", `{"country":"vn","query":"law","as_of":"2024-02-29"}`, true},
		{"invalid date", "SearchInput", `{"country":"vn","query":"law","as_of":"2023-02-29"}`, false},
		{"wrong date format", "SearchInput", `{"country":"vn","query":"law","as_of":"2024-2-09"}`, false},
		{"valid time", "Account", `{"id":1,"email":"x","created_at":"2024-02-29T12:30:59.123+07:00","csrf_token":"x"}`, true},
		{"valid lowercase time", "Account", `{"id":1,"email":"x","created_at":"2024-02-29t12:30:59z","csrf_token":"x"}`, true},
		{"valid leap second", "Account", `{"id":1,"email":"x","created_at":"2016-12-31T23:59:60Z","csrf_token":"x"}`, true},
		{"valid leap second at month end", "Account", `{"id":1,"email":"x","created_at":"2024-03-31T23:59:60Z","csrf_token":"x"}`, true},
		{"invalid leap second", "Account", `{"id":1,"email":"x","created_at":"2024-02-29T12:30:60Z","csrf_token":"x"}`, false},
		{"invalid time", "Account", `{"id":1,"email":"x","created_at":"yesterday","csrf_token":"x"}`, false},
		{"invalid offset", "Account", `{"id":1,"email":"x","created_at":"2024-02-29T12:30:59+24:00","csrf_token":"x"}`, false},
		{"nested invalid date", "SearchResult", `{"abstain":false,"documents":{"vn:1":{"status":{"current":true},"issued":"2024-02-30"}}}`, false},
		{"invalid time in array", "SessionList", `[{"id":1,"created_at":"2024-01-01T00:00:00Z","last_seen_at":"yesterday","current":true}]`, false},
		{"invalid nullable time", "Token", `{"id":1,"name":"x","created_at":"2024-01-01T00:00:00Z","expires_at":"2024-02-01T00:00:00Z","last_used_at":"yesterday","revoked_at":null}`, false},
		{"nullable time", "Token", `{"id":1,"name":"x","created_at":"2024-01-01T00:00:00Z","expires_at":"2024-02-01T00:00:00Z","last_used_at":null,"revoked_at":null}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := decodeJSON([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			err = c.validate(map[string]any{"$ref": "#/components/schemas/" + tc.schema}, value)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestOperationOwnership_CoversContract(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for path, methods := range c.Paths {
		for method, op := range methods {
			if len(op.Tags) != 1 {
				t.Errorf("%s %s has %d owners", method, path, len(op.Tags))
				continue
			}
			switch op.Tags[0] {
			case "weblogin":
				if !strings.HasPrefix(path, "/api/v1/auth/google/") {
					t.Errorf("%s %s is not covered by weblogin", method, path)
				}
			case "websession":
				if path != "/api/v1/auth/logout" {
					t.Errorf("%s %s is not covered by websession", method, path)
				}
			case "password":
				if method != "post" || (path != "/api/v1/auth/password" && path != "/api/v1/me/reauth") {
					t.Errorf("%s %s is not covered by passwordauth", method, path)
				}
			case "account-password":
				if method != "post" {
					t.Errorf("%s %s is not covered by passwordauth", method, path)
				}
				switch path {
				case "/api/v1/auth/signup", "/api/v1/auth/reset", "/api/v1/auth/link", "/api/v1/auth/signup/complete", "/api/v1/auth/reset/complete", "/api/v1/me/password/link", "/api/v1/me/password":
				default:
					t.Errorf("%s %s is not covered by passwordauth", method, path)
				}
			case "factor":
				switch method + " " + path {
				case "get /api/v1/auth/factor", "post /api/v1/auth/factor", "post /api/v1/me/totp/setup", "post /api/v1/me/totp/confirm", "delete /api/v1/me/totp", "post /api/v1/me/recovery-codes":
				default:
					t.Errorf("%s %s is not covered by passwordauth factor handlers", method, path)
				}
			case "account", "web", "reports":
			default:
				t.Errorf("%s %s has unknown owner %q", method, path, op.Tags[0])
			}
		}
	}
}
