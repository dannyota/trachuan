package accountapi

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/websession"

	"gopkg.in/yaml.v3"
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
func TestAccountContract(t *testing.T) {
	cases := []struct{ method, path, actual, body string }{
		{"GET", "/api/v1/me", "/api/v1/me", ""}, {"DELETE", "/api/v1/me", "/api/v1/me", ""},
		{"GET", "/api/v1/me/sessions", "/api/v1/me/sessions", ""}, {"DELETE", "/api/v1/me/sessions/{id}", "/api/v1/me/sessions/1", ""},
		{"GET", "/api/v1/me/tokens", "/api/v1/me/tokens", ""}, {"POST", "/api/v1/me/tokens", "/api/v1/me/tokens", `{"name":"test"}`}, {"DELETE", "/api/v1/me/tokens/{id}", "/api/v1/me/tokens/9", ""}}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			h, _, _, raw, secret := setup(t)
			w := call(h, raw, secret, tc.method, tc.actual, tc.body)
			want := map[string]int{"GET /api/v1/me": 200, "DELETE /api/v1/me": 204, "GET /api/v1/me/sessions": 200, "DELETE /api/v1/me/sessions/{id}": 204, "GET /api/v1/me/tokens": 200, "POST /api/v1/me/tokens": 201, "DELETE /api/v1/me/tokens/{id}": 404}[tc.method+" "+tc.path]
			if w.Code != want {
				t.Fatalf("status %d, want %d", w.Code, want)
			}
			checkContract(t, tc.method, tc.path, w)
		})
	}
}

func TestAccountContract_Guards(t *testing.T) {
	cases := []struct{ method, path, actual string }{
		{"GET", "/api/v1/me", "/api/v1/me"}, {"DELETE", "/api/v1/me", "/api/v1/me"},
		{"GET", "/api/v1/me/sessions", "/api/v1/me/sessions"}, {"DELETE", "/api/v1/me/sessions/{id}", "/api/v1/me/sessions/1"},
		{"GET", "/api/v1/me/tokens", "/api/v1/me/tokens"}, {"POST", "/api/v1/me/tokens", "/api/v1/me/tokens"}, {"DELETE", "/api/v1/me/tokens/{id}", "/api/v1/me/tokens/1"},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			h, _, _, raw, _ := setup(t)
			w := call(h, "invalid", "", tc.method, tc.actual, "")
			if w.Code != 401 {
				t.Fatalf("status %d", w.Code)
			}
			checkContract(t, tc.method, tc.path, w)
			if tc.method != "GET" {
				w = call(h, raw, "invalid", tc.method, tc.actual, "")
				if w.Code != 403 {
					t.Fatalf("status %d", w.Code)
				}
				checkContract(t, tc.method, tc.path, w)
			}
		})
	}
}

func TestAccountContract_Errors(t *testing.T) {
	cases := []struct{ method, path, actual, body string }{
		{"GET", "/api/v1/me", "/api/v1/me", ""}, {"DELETE", "/api/v1/me", "/api/v1/me", ""},
		{"GET", "/api/v1/me/sessions", "/api/v1/me/sessions", ""}, {"DELETE", "/api/v1/me/sessions/{id}", "/api/v1/me/sessions/1", ""},
		{"GET", "/api/v1/me/tokens", "/api/v1/me/tokens", ""}, {"POST", "/api/v1/me/tokens", "/api/v1/me/tokens", `{"name":"test"}`}, {"DELETE", "/api/v1/me/tokens/{id}", "/api/v1/me/tokens/1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			h, s, _, raw, secret := setup(t)
			for range 120 {
				w := call(h, raw, secret, "GET", "/api/v1/me", "")
				if w.Code != 200 {
					t.Fatal(w.Code)
				}
			}
			w := call(h, raw, secret, tc.method, tc.actual, tc.body)
			if w.Code != 429 || w.Header().Get("Retry-After") == "" {
				t.Fatalf("status %d", w.Code)
			}
			checkContract(t, tc.method, tc.path, w)
			m := &websession.Manager{Store: s, Clock: func() time.Time { return s.now }, PublicURL: "https://example.invalid"}
			bad := m.Middleware(m.CSRF(New(Config{})))
			w = call(bad, raw, secret, tc.method, tc.actual, tc.body)
			if w.Code != 503 {
				t.Fatalf("status %d", w.Code)
			}
			checkContract(t, tc.method, tc.path, w)
		})
	}
	for _, tc := range []struct{ path, actual string }{{"/api/v1/me/sessions/{id}", "/api/v1/me/sessions/99"}, {"/api/v1/me/tokens/{id}", "/api/v1/me/tokens/99"}} {
		h, _, _, raw, secret := setup(t)
		w := call(h, raw, secret, "DELETE", tc.actual, "")
		if w.Code != 404 {
			t.Fatal(w.Code)
		}
		checkContract(t, "DELETE", tc.path, w)
	}
	for _, tc := range []struct {
		body   string
		status int
	}{{"invalid", 400}, {`{"name":"` + strings.Repeat("x", (64<<10)+1) + `"}`, 413}} {
		h, _, _, raw, secret := setup(t)
		w := call(h, raw, secret, "POST", "/api/v1/me/tokens", tc.body)
		if w.Code != tc.status {
			t.Fatal(w.Code)
		}
		checkContract(t, "POST", "/api/v1/me/tokens", w)
	}
	h, _, _, raw, secret := setup(t)
	call(h, raw, secret, "POST", "/api/v1/me/tokens", `{"name":"test"}`)
	w := call(h, raw, secret, "DELETE", "/api/v1/me/tokens/1", "")
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	checkContract(t, "DELETE", "/api/v1/me/tokens/{id}", w)
}
