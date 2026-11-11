package websession

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

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
func TestLogoutContract(t *testing.T) {
	m, _, raw := harness()
	r := request(raw)
	r.Method = "POST"
	r.Header.Set("Origin", "https://example.invalid")
	r.Header.Set("X-CSRF-Token", rawValue(2))
	w := httptest.NewRecorder()
	m.Middleware(m.CSRF(http.HandlerFunc(m.Logout))).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
	checkContract(t, "POST", "/api/v1/auth/logout", w)
}

func TestLogoutContract_Errors(t *testing.T) {
	for _, status := range []int{401, 403, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			m, s, raw := harness()
			r := request(raw)
			r.Method = "POST"
			r.Header.Set("Origin", "https://example.invalid")
			r.Header.Set("X-CSRF-Token", rawValue(2))
			if status == 401 {
				r.Header.Del("Cookie")
			}
			if status == 403 {
				r.Header.Del("X-CSRF-Token")
			}
			if status == 503 {
				s.fail = "revoke"
			}
			w := httptest.NewRecorder()
			m.Middleware(m.CSRF(http.HandlerFunc(m.Logout))).ServeHTTP(w, r)
			if w.Code != status {
				t.Fatalf("status %d", w.Code)
			}
			checkContract(t, "POST", "/api/v1/auth/logout", w)
		})
	}
}
