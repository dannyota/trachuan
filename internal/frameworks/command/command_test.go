package command

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_SyntheticSourceOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sources"), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"catalog":{"groups":[{"id":"private-citation","title":"invented-private-title","controls":[]}]}}`)
	sum := sha256.Sum256(data)
	if err := os.WriteFile(filepath.Join(dir, "fixture.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	registry := fmt.Sprintf("frameworks:\n  - code: alpha\n    version: '1.0'\n    name: Invented\n    publisher: Invented\n    language: en\n    access: licensed\n    ordinal: 1\n    citation_scheme: invented\n    parser: nist-oscal-800-53\n    expected_controls: 1\n    files:\n      - path: fixture.json\n        sha256: %x\n        size: %d\n        role: catalog\n        format: oscal-json\n", sum, len(data))
	if err := os.WriteFile(filepath.Join(dir, "sources", "frameworks.yaml"), []byte(registry), 0600); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Error(err)
		}
	})
	t.Setenv("FRAMEWORKS_SOURCE_DIR", dir)
	var out bytes.Buffer
	if err := Run([]string{"alpha", "1.0"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Run([]string{"edges"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "resolved: 0") || strings.Contains(out.String(), "private-citation") || strings.Contains(out.String(), "invented-private-title") {
		t.Fatal("edge statistics leaked text or omitted counts")
	}
	out.Reset()
	if err := Run([]string{"alpha", "1.0"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-citation", "invented-private-title"} {
		if strings.Contains(strings.ToLower(out.String()), secret) {
			t.Error("source text in output")
		}
	}
	if !strings.Contains(out.String(), "controls: 1") {
		t.Error("missing control count")
	}
	for _, replacement := range []string{"expected_controls: 2", ""} {
		changed := strings.Replace(registry, "expected_controls: 1", replacement, 1)
		if err := os.WriteFile(filepath.Join(dir, "sources", "frameworks.yaml"), []byte(changed), 0600); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		err := Run([]string{"alpha", "1.0"}, &out)
		want := "control count mismatch"
		if replacement == "" {
			want = "expected control count unset"
		}
		if err == nil || err.Error() != want {
			t.Errorf("unexpected error class: %v", err)
		}
		if !strings.Contains(out.String(), "controls: 1") {
			t.Error("missing count on failure")
		}
	}
	for _, tc := range []struct{ parser, content, want string }{
		{"nist-oscal-800-53", `{"catalog":"invented-private-value"}`, "invalid catalog JSON"},
		{"cis-controls-8-xlsx", "invented-private-value", "invalid workbook"},
	} {
		broken := []byte(tc.content)
		brokenSum := sha256.Sum256(broken)
		if err := os.WriteFile(filepath.Join(dir, "fixture.json"), broken, 0600); err != nil {
			t.Fatal(err)
		}
		revised := strings.Replace(registry, fmt.Sprintf("sha256: %x", sum), fmt.Sprintf("sha256: %x", brokenSum), 1)
		revised = strings.Replace(revised, fmt.Sprintf("size: %d", len(data)), fmt.Sprintf("size: %d", len(broken)), 1)
		revised = strings.Replace(revised, "nist-oscal-800-53", tc.parser, 1)
		if err := os.WriteFile(filepath.Join(dir, "sources", "frameworks.yaml"), []byte(revised), 0600); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		err := Run([]string{"alpha", "1.0"}, &out)
		if err == nil || err.Error() != tc.want {
			t.Errorf("unexpected error class: %v", err)
		}
		if strings.Contains(out.String(), "invented-private-value") {
			t.Error("source text in output")
		}
	}

}

func TestRun_UsageNamesBothForms(t *testing.T) {
	err := Run(nil, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "frameworks <code> <version>") || !strings.Contains(err.Error(), "frameworks edges") {
		t.Fatal("usage omits command forms")
	}
}
