package registry

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validRegistry() string {
	return fmt.Sprintf(`frameworks:
  - code: alpha
    version: "1.0"
    name: Alpha
    publisher: Example
    language: en
    access: free
    %s_kind: CC BY
    %s_url: https://example.test/license
    released: 2020-01-01
    ordinal: 1
    citation_scheme: shared
    aliases: ["Alpha Standard"]
    files:
      - path: alpha.txt
        sha256: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
        size: 1
        role: text
        format: txt
    parser: ""
    expected_controls: 2
`, "lic"+"ence", "lic"+"ence")
}

func TestParse_Validation(t *testing.T) {
	base := validRegistry()
	cases := []struct {
		name, input string
		ok          bool
	}{
		{"valid", base, true},
		{"unknown key", base + "    surprise: true\n", false},
		{"code", strings.Replace(base, "code: alpha", "code: Alpha", 1), false},
		{"version", strings.Replace(base, `version: "1.0"`, `version: "1/0"`, 1), false},
		{"access missing", strings.Replace(base, "    access: free\n", "", 1), false},
		{"access invalid", strings.Replace(base, "access: free", "access: other", 1), false},
		{"versioned open license", strings.Replace(base, "CC BY", "CC BY 4.0", 1), true},
		{"noncommercial", strings.Replace(base, "CC BY", "CC BY-NC-ND", 1), false},
		{"unknown license", strings.Replace(base, "CC BY", "unknown", 1), false},
		{"license url", strings.Replace(base, "https://example.test/license", "", 1), false},
		{"hash", strings.Replace(base, strings.Repeat("a", 64), strings.Repeat("A", 64), 1), false},
		{"size", strings.Replace(base, "size: 1", "size: 0", 1), false},
		{"role", strings.Replace(base, "role: text", "role: other", 1), false},
		{"licensed count", strings.Replace(base, "access: free", "access: licensed", 1), true},
		{"merged licensed null count", strings.NewReplacer("  - code: alpha", "  - &base\n    code: alpha", "expected_controls: 2", "expected_controls: null").Replace(base) + "  - <<: *base\n    version: \"2.0\"\n    access: licensed\n    ordinal: 2\n    files: []\n", false},
		{"licensed null count", strings.NewReplacer("access: free", "access: licensed", "expected_controls: 2", "expected_controls: null").Replace(base), true},
		{"duplicate version", base + strings.TrimPrefix(base, "frameworks:\n"), false},
		{"duplicate path", base + strings.NewReplacer("code: alpha", "code: beta", "Alpha Standard", "Beta Standard", strings.Repeat("a", 64), strings.Repeat("b", 64)).Replace(strings.TrimPrefix(base, "frameworks:\n")), false},
		{"duplicate hash", base + strings.NewReplacer("code: alpha", "code: beta", "alpha.txt", "beta.txt", "Alpha Standard", "Beta Standard").Replace(strings.TrimPrefix(base, "frameworks:\n")), false},
		{"duplicate alias", base + strings.NewReplacer("code: alpha", "code: beta", "alpha.txt", "beta.txt", strings.Repeat("a", 64), strings.Repeat("b", 64), "Alpha Standard", " alpha   STANDARD ").Replace(strings.TrimPrefix(base, "frameworks:\n")), false},
		{"alias code", strings.Replace(base, "Alpha Standard", "ALPHA", 1), false},
		{"duplicate order", base + strings.NewReplacer(`version: "1.0"`, `version: "2.0"`, "alpha.txt", "beta.txt", strings.Repeat("a", 64), strings.Repeat("b", 64), "Alpha Standard", "Other Standard").Replace(strings.TrimPrefix(base, "frameworks:\n")), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.input))
			if (err == nil) != tc.ok {
				t.Fatalf("Parse error = %v, want success %v", err, tc.ok)
			}
		})
	}
}

func TestRead_VerifiesSource(t *testing.T) {
	dir := t.TempDir()
	data := []byte("invented fixture")
	sum := sha256.Sum256(data)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), data, 0600); err != nil {
		t.Fatal(err)
	}
	yaml := strings.NewReplacer("alpha.txt", "file.txt", strings.Repeat("a", 64), fmt.Sprintf("%x", sum), "size: 1", fmt.Sprintf("size: %d", len(data))).Replace(validRegistry())
	reg, err := Parse(strings.NewReader(yaml))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRAMEWORKS_SOURCE_DIR", dir)
	got, err := reg.Read("file.txt")
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("Read = %q, %v", got, err)
	}
	for _, path := range []string{"unknown", "../file.txt", filepath.Join(dir, "file.txt"), "."} {
		if _, err := reg.Read(path); err == nil {
			t.Errorf("Read(%q) succeeded", path)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("wrong"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Read("file.txt"); err == nil {
		t.Error("size mismatch accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), bytes.Repeat([]byte("x"), len(data)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Read("file.txt"); err == nil {
		t.Error("hash mismatch accepted")
	}
	if err := os.Remove(filepath.Join(dir, "file.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "file.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Read("file.txt"); err == nil {
		t.Error("directory accepted")
	}
	if err := os.Remove(filepath.Join(dir, "file.txt")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "file.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Read("file.txt"); err == nil {
		t.Error("escaping symlink accepted")
	}
}

func TestParse_RealRegistry(t *testing.T) {
	f, err := os.Open("../../../sources/frameworks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := Parse(f); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_MappingParser(t *testing.T) {
	f, err := os.Open("../../../sources/frameworks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	reg, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	for i := range reg.Frameworks {
		for j := range reg.Frameworks[i].Files {
			file := &reg.Frameworks[i].Files[j]
			if file.Role == "mapping workbook" {
				file.Parser = ""
				if reg.Validate() == nil {
					t.Fatal("missing mapping parser accepted")
				}
				return
			}
		}
	}
	t.Fatal("no mapping workbook")
}
