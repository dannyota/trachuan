package evaluation

import (
	"encoding/json"
	"testing"
)

func TestReport_TextSourceCounts(t *testing.T) {
	input := []byte(`{"documents_by_text_source":{"source_tree":2,"html_transcription":1},"documents_by_text_source_and_class":{"source_tree":{"primary":1,"relation_context":1},"html_transcription":{"primary":1}},"documents_text_incomplete":1}`)
	var report Report
	if err := json.Unmarshal(input, &report); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"documents_by_text_source", "documents_by_text_source_and_class", "documents_text_incomplete"} {
		var wantValue, gotValue any
		if err = json.Unmarshal(inputMap(t, input)[key], &wantValue); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(got[key], &gotValue); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		x, _ := json.Marshal(wantValue)
		y, _ := json.Marshal(gotValue)
		if string(x) != string(y) {
			t.Errorf("%s=%s want %s", key, y, x)
		}
	}
}
func inputMap(t *testing.T, b []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
