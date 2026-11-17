package publicsummary

import (
	"encoding/json"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/contracttest"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestSummaryContract_RejectsExtraFieldsAtEveryLevel(t *testing.T) {
	h, _, _, _ := handlerFixture(t)
	contract, err := contracttest.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, schema string
		objects      [][]any
	}{
		{"/api/v1/public/documents/vn:vbpl:1", "PublicSummary", [][]any{{}, {"document"}, {"document", "status"}, {"relations", 0}, {"nodes", 0}, {"outline", 0}}},
		{"/api/v1/public/documents", "PublicSummaryList", [][]any{{}, {"items", 0}}},
	} {
		raw, encodeErr := json.Marshal(map[string]any{"$defs": contract.Components.Schemas, "allOf": []any{map[string]any{"$ref": "#/$defs/" + tc.schema}}})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		var schema jsonschema.Schema
		if decodeErr := json.Unmarshal([]byte(strings.ReplaceAll(string(raw), "#/components/schemas/", "#/$defs/")), &schema); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		resolved, resolveErr := schema.Resolve(nil)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		response := summaryCall(h, tc.path)
		if response.Code != 200 {
			t.Fatal(response.Code, response.Body.String())
		}
		for _, path := range tc.objects {
			var value any
			if decodeErr := json.Unmarshal(response.Body.Bytes(), &value); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if validationErr := resolved.Validate(value); validationErr != nil {
				t.Fatal("actual response rejected", validationErr)
			}
			object := value
			for _, part := range path {
				switch key := part.(type) {
				case string:
					object = object.(map[string]any)[key]
				case int:
					object = object.([]any)[key]
				}
			}
			object.(map[string]any)["provision_text"] = "forbidden marker"
			if validationErr := resolved.Validate(value); validationErr == nil {
				t.Fatalf("%s permits extra field at %v", tc.schema, path)
			}
		}
	}
}
