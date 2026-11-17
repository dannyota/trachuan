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

func TestTaxonomyFreeze_CarriesLabelerFragments(t *testing.T) {
	dir := t.TempDir()
	packet := `{"country":"vn","release":"20990101","slice":"post-2023","draw_rule":"rule","labeler_fragments":3,"articles":[{"document_key":"doc","anchor":"dieu-1","text":"text","own_text_sha256":"` + taxonomy.OwnTextSHA256("text") + `"}],"tree":[{"code":"1.1","level":2}]}`
	labels := `{"articles":[{"document_key":"doc","anchor":"dieu-1","codes":["1.1"]}]}`
	p, l, out := filepath.Join(dir, "packet.json"), filepath.Join(dir, "labels.json"), filepath.Join(dir, "taxonomy-post-2023.json")
	if err := os.WriteFile(p, []byte(packet), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l, []byte(labels), 0600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := runTaxonomyFreeze([]string{"--packet", p, "--labels", l, "--labeler", "gpt-6-astra", "--out", out}, &stderr); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var frozen struct {
		Count int `json:"labeler_fragments"`
	}
	if err = json.Unmarshal(data, &frozen); err != nil {
		t.Fatal(err)
	}
	if frozen.Count != 3 {
		t.Fatalf("got %d", frozen.Count)
	}
}
