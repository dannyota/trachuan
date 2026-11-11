package webapi

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"danny.vn/trachuan/apps/server/internal/release"
)

func checkContract(t *testing.T, method, path string, w *httptest.ResponseRecorder) {
	t.Helper()
	raw, err := os.ReadFile("../../../../packages/schema/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]map[string]any `yaml:"schemas"`
		} `yaml:"components"`
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	op, ok := doc.Paths[path][strings.ToLower(method)]
	if !ok {
		t.Fatalf("missing operation %s %s", method, path)
	}
	response, ok := op.Responses[strconv.Itoa(w.Code)]
	if !ok {
		t.Fatalf("undocumented %s %s status %d: %s", method, path, w.Code, w.Body.String())
	}
	if len(w.Body.Bytes()) == 0 {
		if len(response.Content) != 0 {
			t.Fatalf("documented content for empty response")
		}
		return
	}
	media := strings.Split(w.Header().Get("Content-Type"), ";")[0]
	content, ok := response.Content[media]
	if !ok {
		t.Fatalf("undocumented media %s", media)
	}
	if media == "application/json" {
		var payload any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if content.Schema.Ref == "" {
			t.Fatal("missing schema reference")
		}
		validateShape(t, doc.Components.Schemas, map[string]any{"$ref": content.Schema.Ref}, payload)
	}
}
func validateShape(t *testing.T, schemas map[string]map[string]any, schema map[string]any, value any) {
	t.Helper()
	if ref, ok := schema["$ref"].(string); ok {
		schema = schemas[strings.TrimPrefix(ref, "#/components/schemas/")]
		if schema == nil {
			t.Fatalf("unknown schema %s", ref)
		}
	}
	if value == nil {
		if types, ok := schema["type"].([]any); ok {
			for _, typ := range types {
				if typ == "null" {
					return
				}
			}
		}
		t.Fatal("unexpected null")
	}
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("want object, got %T", value)
		}
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				if _, exists := object[key.(string)]; !exists {
					t.Fatalf("missing %s", key)
				}
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for key, item := range object {
			if prop, ok := properties[key].(map[string]any); ok {
				validateShape(t, schemas, prop, item)
			} else if extra, ok := schema["additionalProperties"].(map[string]any); ok {
				validateShape(t, schemas, extra, item)
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			t.Fatalf("want array, got %T", value)
		}
		child, _ := schema["items"].(map[string]any)
		for _, item := range items {
			validateShape(t, schemas, child, item)
		}
	case "string":
		if _, ok := value.(string); !ok {
			t.Fatalf("want string, got %T", value)
		}
	case "integer", "number":
		if _, ok := value.(float64); !ok {
			t.Fatalf("want number, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			t.Fatalf("want bool, got %T", value)
		}
	}
}
func TestWebContract(t *testing.T) {
	cases := []struct{ method, path, actual, body string }{
		{"POST", "/api/v1/search", "/api/v1/search", `{"country":"vn","query":"law"}`}, {"POST", "/api/v1/read", "/api/v1/read", `{"country":"vn","items":[{"handle":"vn:1#a"}]}`}, {"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1", ""}, {"GET", "/api/v1/status", "/api/v1/status", ""}}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			w := webCall(newHarness(t), tc.method, tc.actual, tc.body)
			if w.Code != 200 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			checkContract(t, tc.method, tc.path, w)
		})
	}
}

func TestWebContract_Guards(t *testing.T) {
	cases := []struct{ method, path, actual string }{
		{"POST", "/api/v1/search", "/api/v1/search"}, {"POST", "/api/v1/read", "/api/v1/read"}, {"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1"}, {"GET", "/api/v1/status", "/api/v1/status"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			h := newHarness(t)
			r := webRequest(tc.method, tc.actual, "")
			r.Header.Del("Cookie")
			w := httptest.NewRecorder()
			h.web.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatalf("status %d", w.Code)
			}
			checkContract(t, tc.method, tc.path, w)
			if tc.method == "POST" {
				r = webRequest(tc.method, tc.actual, "")
				r.Header.Del("X-CSRF-Token")
				w = httptest.NewRecorder()
				h.web.ServeHTTP(w, r)
				if w.Code != 403 {
					t.Fatalf("status %d", w.Code)
				}
				checkContract(t, tc.method, tc.path, w)
			}
		})
	}
}

func TestWebContract_Errors(t *testing.T) {
	cases := []struct{ method, path, actual, body string }{
		{"POST", "/api/v1/search", "/api/v1/search", `{"country":"vn","query":"law"}`}, {"POST", "/api/v1/read", "/api/v1/read", `{"country":"vn","items":[]}`}, {"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1", ""}, {"GET", "/api/v1/status", "/api/v1/status", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			h := newHarness(t)
			h.resolver.err = release.ErrExpired
			w := webCall(h, tc.method, tc.actual, tc.body)
			if w.Code != 409 {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			checkContract(t, tc.method, tc.path, w)
			bad := webCall(newHarness(t), tc.method, tc.actual+"?other=x", "invalid")
			if tc.method == "POST" {
				bad = webCall(newHarness(t), tc.method, tc.actual, "invalid")
			}
			if bad.Code != 400 {
				t.Fatalf("status %d: %s", bad.Code, bad.Body.String())
			}
			checkContract(t, tc.method, tc.path, bad)
			unavailable := httptest.NewRecorder()
			New(Config{}).ServeHTTP(unavailable, httptest.NewRequest(tc.method, tc.actual, nil))
			if unavailable.Code != 503 {
				t.Fatal(unavailable.Code)
			}
			checkContract(t, tc.method, tc.path, unavailable)
		})
	}
}

func TestWebContract_BodyAndRateLimits(t *testing.T) {
	for _, tc := range []struct {
		method, path, actual, body string
		count                      int
	}{
		{"POST", "/api/v1/search", "/api/v1/search", `{"country":"vn","query":"law"}`, 60},
		{"POST", "/api/v1/read", "/api/v1/read", `{"country":"vn","items":[]}`, 120},
		{"GET", "/api/v1/documents/{key}", "/api/v1/documents/vn:1", "", 120},
		{"GET", "/api/v1/status", "/api/v1/status", "", 120},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if tc.method == "POST" {
				w := webCall(newHarness(t), tc.method, tc.actual, strings.Repeat("x", (64<<10)+1))
				if w.Code != 413 {
					t.Fatal(w.Code)
				}
				checkContract(t, tc.method, tc.path, w)
			}
			h := newHarness(t)
			var w *httptest.ResponseRecorder
			for i := 0; i <= tc.count; i++ {
				w = webCall(h, tc.method, tc.actual, tc.body)
			}
			if w.Code != 429 || w.Header().Get("Retry-After") == "" {
				t.Fatalf("status %d headers %v", w.Code, w.Header())
			}
			checkContract(t, tc.method, tc.path, w)
		})
	}
}
