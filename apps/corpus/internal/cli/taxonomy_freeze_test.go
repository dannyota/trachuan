package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestTaxonomyFreeze_WritesFrozenSet(t *testing.T) {
	dir := t.TempDir()
	packet := taxonomy.Packet{Country: "vn", Release: "20990101", Slice: "post-2023", DrawRule: "rule", Articles: []taxonomy.PacketArticle{{Key: "doc", Anchor: "dieu-1", Text: "text", OwnTextSHA256: taxonomy.OwnTextSHA256("text")}}, Tree: []taxonomy.PacketNode{{Code: "1.1", Level: 2}}}
	input := func(name string, v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err = os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := input("packet.json", packet)
	l := input("labels.json", map[string]any{"articles": []taxonomy.LabelArticle{{Key: "doc", Anchor: "dieu-1", Codes: []string{"1.1"}}}})
	out := filepath.Join(dir, "taxonomy-post-2023.json")
	var stderr bytes.Buffer
	if code := runTaxonomyFreeze([]string{"--packet", p, "--labels", l, "--labeler", "gpt-6-astra", "--out", out}, &stderr); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}
