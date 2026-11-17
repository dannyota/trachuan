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

func TestTaxonomyFragments_CountsFrozenArticles(t *testing.T) {
	var labels taxonomy.Labels
	labels.Country, labels.Release = "vn", "20990101"
	var evidence drawEvidence
	for i := 0; i < 300; i++ {
		text := "Điều 1. Heading\nNgười dân được cấp phép."
		if i == 0 {
			text = "Điều 1. Heading\nThông tư này có hiệu lực thi hành từ ngày 1 tháng 1 năm 2027."
		}
		key := fmt.Sprintf("doc-%d", i)
		labels.Articles = append(labels.Articles, taxonomy.LabelArticle{Key: key, Anchor: "dieu-1", OwnTextSHA256: taxonomy.OwnTextSHA256(text)})
		evidence.articles = append(evidence.articles, taxonomy.DrawArticle{PacketArticle: taxonomy.PacketArticle{Key: key, Anchor: "dieu-1", Text: text}})
	}
	data, _ := json.Marshal(labels)
	path := t.TempDir() + "/labels.json"
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	load := func(context.Context, string, string) (drawEvidence, error) { return evidence, nil }
	var out, stderr bytes.Buffer
	if code := runTaxonomyFragmentsWith(context.Background(), []string{"--labels", path, "vn", "20990101"}, &out, &stderr, load); code != 0 || !strings.Contains(out.String(), "1") {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), stderr.String())
	}
}
