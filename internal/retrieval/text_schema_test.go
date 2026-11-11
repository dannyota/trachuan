package retrieval

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTextSchema_DocumentAndLocators(t *testing.T) {
	b, err := os.ReadFile("../../packages/schema/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	for _, tc := range []struct {
		schema, property, kind string
		required               bool
	}{{"Document", "text_source", "string", true}, {"Document", "text_complete", "boolean", true}, {"Hit", "locator", "string", false}, {"Unit", "locator", "string", false}} {
		schema := schemas[tc.schema].(map[string]any)
		properties := schema["properties"].(map[string]any)
		property, _ := properties[tc.property].(map[string]any)
		if property["type"] != tc.kind {
			t.Errorf("%s.%s type=%v", tc.schema, tc.property, property["type"])
		}
		required := false
		if values, ok := schema["required"].([]any); ok {
			for _, value := range values {
				if value == tc.property {
					required = true
				}
			}
		}
		if required != tc.required {
			t.Errorf("%s.%s required=%v", tc.schema, tc.property, required)
		}
	}
}
