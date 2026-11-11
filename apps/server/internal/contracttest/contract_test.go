package contracttest

import (
	"net/http/httptest"
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
