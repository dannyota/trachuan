package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestParseDate_FormatsAndRejects(t *testing.T) {
	tests := []struct {
		input, want string
		invalid     bool
	}{
		{"", "", false}, {" 2024-02-29 ", "2024-02-29", false},
		{"29/02/2024", "2024-02-29", false},
		{"2024-01-01T20:00:00Z", "2024-01-02", false},
		{"1900-01-01T16:55:00Z", "1900-01-02", false},
		{"2023-02-29", "", true}, {"0000-01-01", "", true},
		{"2024-01-01T12:00:00", "", true}, {"2024-1-01", "", true},
		{"2020-07-10T00:00:00", "2020-07-10", false}, {"2020-07-10T00:00:01", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDate(tt.input)
			if (err != nil) != tt.invalid {
				t.Fatalf("error = %v, invalid = %v", err, tt.invalid)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if got == nil || got.Format("2006-01-02") != tt.want {
				t.Fatalf("got %v, want %s", got, tt.want)
			}
		})
	}
}

func TestStatus_UnknownCode(t *testing.T) {
	codes := map[string]source.StatusCode{"A": {Class: "in_force", Current: true}}
	for _, tt := range []struct {
		input, class   string
		current, known bool
	}{{" a ", "in_force", true, true}, {"x", "unknown", false, false}, {"", "unknown", false, false}} {
		class, current, known := Status(tt.input, codes)
		if class != tt.class || current != tt.current || known != tt.known {
			t.Errorf("Status(%q) = %q, %v, %v", tt.input, class, current, known)
		}
	}
}

// Recorded VBPL details carry offset-free midnight timestamps; every present
// date in them must parse.
func TestParseDate_RecordedVBPLDetails(t *testing.T) {
	paths, err := filepath.Glob("../../vbpl/testdata/detail-*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("recorded details: %v %v", paths, err)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var detail struct {
			Data map[string]any `json:"data"`
		}
		if err = json.Unmarshal(data, &detail); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"issueDate", "effFrom", "effTo"} {
			raw, ok := detail.Data[field].(string)
			if !ok {
				continue
			}
			if got, err := ParseDate(raw); err != nil || got == nil {
				t.Errorf("%s %s %q: %v", filepath.Base(path), field, raw, err)
			}
		}
	}
}
