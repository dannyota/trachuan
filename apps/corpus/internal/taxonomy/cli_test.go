package taxonomy_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/cli"
)

func TestTaxonomyCLI_ImportAndCheck(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "package.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("jsonData.js")
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Write([]byte(`var jdChuDe = [{"Value":"s","Text":"Subject","STT":"1"}]; var jdDeMuc = [{"Value":"d","ChuDe":"s","Text":"Section","STT":"1"}]; var jdAllTree = [];`))
	if err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	mapping := filepath.Join(dir, "mapping.yaml")
	custom := filepath.Join(dir, "custom.yaml")
	overrides := filepath.Join(dir, "overrides.yaml")
	for _, path := range []string{custom, overrides} {
		if err = os.WriteFile(path, []byte("[]\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errs bytes.Buffer
	run := func(args ...string) int {
		out.Reset()
		errs.Reset()
		return cli.Run(context.Background(), args, &out, &errs)
	}
	if status := run("taxonomy", "import", "--package", zipPath, "--mapping", mapping); status != 0 {
		t.Fatalf("import %d: %s", status, errs.String())
	}
	info, err := os.Stat(mapping)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Fatalf("mapping mode = %04o, want 0644", got)
	}
	if !strings.Contains(out.String(), "active") {
		t.Fatal(out.String())
	}
	if status := run("taxonomy", "check", "--mapping", mapping, "--custom", custom, "--overrides", overrides, "--package", zipPath); status != 0 {
		t.Fatalf("check %d: %s", status, errs.String())
	}
	data, err := os.ReadFile(mapping)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("package_sha256:"), []byte("package_sha256: mismatch #"), 1)
	if err = os.WriteFile(mapping, data, 0600); err != nil {
		t.Fatal(err)
	}
	if status := run("taxonomy", "check", "--mapping", mapping, "--custom", custom, "--overrides", overrides, "--package", zipPath); status == 0 {
		t.Fatal("accepted mismatched hash")
	}
}
