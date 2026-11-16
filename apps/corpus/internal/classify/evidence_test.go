package classify

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFields_VerifiesPinnedArtifactAndSourceIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, body, key, hash string
		fail                  bool
	}{
		{"present", `{"data":{"id":"1","documentFields":[{"name":"Synthetic field"}]}}`, "vn:vbpl:1", "", false},
		{"empty", `{"data":{"id":"1","documentFields":null}}`, "vn:vbpl:1", "", false},
		{"missing", `{"data":{"id":"1"}}`, "vn:vbpl:1", "", true},
		{"wrong id", `{"data":{"id":"2","documentFields":[]}}`, "vn:vbpl:1", "", true},
		{"wrong hash", `{"data":{"id":"1","documentFields":[]}}`, "vn:vbpl:1", "wrong", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "detail.json"), []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			manifest := []byte(fmt.Sprintf("%s  detail.json\n", digest([]byte(tc.body))))
			if err := os.WriteFile(filepath.Join(dir, "MANIFEST.sha256"), manifest, 0600); err != nil {
				t.Fatal(err)
			}
			h := digest(manifest)
			if tc.hash != "" {
				h = tc.hash
			}
			fields, err := ReadFields(dir, tc.key, h)
			if (err != nil) != tc.fail {
				t.Fatalf("fields %v error %v", fields, err)
			}
			if tc.name == "present" && (len(fields) != 1 || fields[0] != "Synthetic field") {
				t.Fatal("lost VBPL field")
			}
		})
	}
}
