package knowledge

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSources_EveryKnowledgeFileParses(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "sources")
	paths, err := filepath.Glob(filepath.Join(root, "*-knowledge.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range paths {
		country := strings.TrimSuffix(filepath.Base(path), "-knowledge.yaml")
		found = found || country == "vn"
		if _, _, err := ParseFile(country, path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	if !found {
		t.Fatal("sources/vn-knowledge.yaml is missing")
	}
}
