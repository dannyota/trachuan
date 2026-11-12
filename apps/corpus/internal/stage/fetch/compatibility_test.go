package fetch

import (
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestFetch_Compatibility(t *testing.T) {
	s, env := setup(t, &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}})
	if got := s.Recipe(stage.Snapshot{}); got != "1f6699b0264c7ff2550542ccf2689868c1d40e435464151edcfa3dd991b9b351" {
		t.Fatalf("recipe changed: %s", got)
	}
	out := process(t, s, env)
	if out.Artifact.SHA256 != "3e563c9ecaaf29e3306c27843063466f04a79ca5b11c9f069187a7ab198abc42" {
		t.Fatalf("recorded bundle changed: %s", out.Artifact.SHA256)
	}
	for name, want := range map[string]string{
		"files.json":  `[{"name":"a","sha256":"7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed","size":3}]`,
		"report.json": `{"missing":[],"too_large":[]}`,
	} {
		got, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %s %v", name, got, err)
		}
	}
}
