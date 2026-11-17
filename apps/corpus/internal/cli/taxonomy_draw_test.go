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

func TestTaxonomyDraw_SkipsFragmentsAndIncludesRule(t *testing.T) {
	var stderr bytes.Buffer
	e := drawEvidence{}
	for i := 0; i < 120; i++ {
		text := "Điều 1. Quy định\nNgười dân được cấp giấy phép."
		if i < 20 {
			text = "Điều 1. Hiệu lực\nThông tư này có hiệu lực thi hành từ ngày 1 tháng 1 năm 2027."
		}
		e.articles = append(e.articles, taxonomy.DrawArticle{PacketArticle: taxonomy.PacketArticle{Key: fmt.Sprintf("doc-%d", i), Anchor: "dieu-1", Text: text}, Primary: true, IssueDate: "2024-01-01"})
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
	var packet struct {
		Articles []taxonomy.PacketArticle `json:"articles"`
		Skipped  int                      `json:"fragments_skipped"`
		Rule     string                   `json:"labeling_rule"`
	}
	if err := json.Unmarshal(data, &packet); err != nil {
		t.Fatal(err)
	}
	if len(packet.Articles) != 100 || packet.Skipped != 20 || !strings.Contains(packet.Rule, "Assign a node only when") {
		t.Fatalf("invalid packet: %s", data)
	}
	for _, a := range packet.Articles {
		if taxonomy.IsFragment(a.Text) {
			t.Fatal("fragment selected")
		}
	}
}

func TestTaxonomyDraw_ExclusionReplacesOnlySelectedArticles(t *testing.T) {
	e := drawEvidence{}
	for i := 0; i < 120; i++ {
		for j := 0; j < 3; j++ {
			e.articles = append(e.articles, taxonomy.DrawArticle{PacketArticle: taxonomy.PacketArticle{Key: fmt.Sprintf("doc-%03d", i), Anchor: fmt.Sprintf("dieu-%d", j), Text: "operative"}, Primary: true, IssueDate: "2024-01-01"})
		}
	}
	load := func(context.Context, string, string) (drawEvidence, error) { return e, nil }
	dir := t.TempDir()
	draw := func(name string, extra ...string) struct {
		Articles []taxonomy.PacketArticle `json:"articles"`
		Skipped  int                      `json:"fragments_skipped"`
		Labeler  int                      `json:"labeler_fragments"`
	} {
		t.Helper()
		path := dir + "/" + name + ".json"
		args := append([]string{"vn", "20990101", "--slice", "post-2023", "--seed", "7", "--out", path}, extra...)
		var stderr bytes.Buffer
		if code := runTaxonomyDrawWith(context.Background(), args, &stderr, load); code != 0 {
			t.Fatalf("draw: %d %s", code, stderr.String())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var packet struct {
			Articles []taxonomy.PacketArticle `json:"articles"`
			Skipped  int                      `json:"fragments_skipped"`
			Labeler  int                      `json:"labeler_fragments"`
		}
		if err = json.Unmarshal(data, &packet); err != nil {
			t.Fatal(err)
		}
		return packet
	}
	original := draw("original")
	excluded := original.Articles[0]
	data, err := json.Marshal(map[string]any{"articles": []any{map[string]string{"document_key": excluded.Key, "anchor": excluded.Anchor}}})
	if err != nil {
		t.Fatal(err)
	}
	path := dir + "/excluded.json"
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	replacement := draw("replacement", "--exclude", path)
	if len(replacement.Articles) != 100 || replacement.Labeler != 1 || replacement.Skipped != original.Skipped {
		t.Fatalf("counts: %d %d %d", len(replacement.Articles), replacement.Labeler, replacement.Skipped)
	}
	seen := map[string]bool{}
	for _, a := range replacement.Articles {
		seen[a.Key+"/"+a.Anchor] = true
	}
	for _, a := range original.Articles {
		id := a.Key + "/" + a.Anchor
		if a.Key == excluded.Key && a.Anchor == excluded.Anchor {
			if seen[id] {
				t.Fatal("excluded article remains")
			}
			continue
		}
		if !seen[id] {
			t.Fatalf("original article lost: %s", id)
		}
	}
}

func TestTaxonomyDraw_RejectsMalformedExclusionFile(t *testing.T) {
	var rows []taxonomy.DrawArticle
	for i := 0; i < 120; i++ {
		rows = append(rows, taxonomy.DrawArticle{PacketArticle: taxonomy.PacketArticle{Key: fmt.Sprintf("doc-%03d", i), Anchor: "dieu-1", Text: "operative"}, Primary: true, IssueDate: "2024-01-01"})
	}
	load := func(context.Context, string, string) (drawEvidence, error) { return drawEvidence{articles: rows}, nil }
	dir := t.TempDir()
	path := dir + "/exclude.json"
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := runTaxonomyDrawWith(context.Background(), []string{"vn", "20990101", "--slice", "post-2023", "--seed", "7", "--out", dir + "/out.json", "--exclude", path}, &stderr, load); code == 0 {
		t.Fatal("missing articles accepted")
	}
}
