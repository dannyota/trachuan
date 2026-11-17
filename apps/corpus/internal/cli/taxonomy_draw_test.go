package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestTaxonomyDraw_RefusesManualAssignments(t *testing.T) {
	var stderr bytes.Buffer
	load := func(context.Context, string, string) (drawEvidence, error) { return drawEvidence{manualCount: 3}, nil }
	code := runTaxonomyDrawWith(context.Background(), []string{"vn", "20990101", "--slice", "post-2023", "--seed", "1", "--out", t.TempDir() + "/packet.json"}, &stderr, load)
	if code == 0 || !strings.Contains(stderr.String(), "3") {
		t.Fatalf("code=%d error=%s", code, stderr.String())
	}
}

func TestTaxonomyDraw_PacketOmitsClassification(t *testing.T) {
	var stderr bytes.Buffer
	e := drawEvidence{}
	for i := 0; i < 100; i++ {
		e.articles = append(e.articles, taxonomy.DrawArticle{PacketArticle: taxonomy.PacketArticle{Key: fmt.Sprintf("doc-%d", i), Anchor: "dieu-1", Text: "text"}, Primary: true, IssueDate: "2024-01-01"})
	}
	load := func(context.Context, string, string) (drawEvidence, error) { return e, nil }
	path := t.TempDir() + "/packet.json"
	if code := runTaxonomyDrawWith(context.Background(), []string{"vn", "20990101", "--slice", "post-2023", "--seed", "1", "--out", path}, &stderr, load); code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var packet taxonomy.Packet
	if err = json.Unmarshal(data, &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.Articles) != 100 || packet.Articles[0].Text != "text" || packet.Articles[0].OwnTextSHA256 != taxonomy.OwnTextSHA256("text") {
		t.Fatalf("invalid packet %s", data)
	}
	for _, word := range []string{"seed_codes", "inherited", "classifier", "codes"} {
		if strings.Contains(string(data), word) {
			t.Fatalf("packet contains %s", word)
		}
	}
}
