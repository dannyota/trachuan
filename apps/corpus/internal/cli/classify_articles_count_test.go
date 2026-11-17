package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestClassifyArticles_LabeledCountBounds(t *testing.T) {
	for _, tc := range []struct {
		count  int
		loaded bool
	}{{1, true}, {100, true}, {300, true}, {301, false}} {
		dir := t.TempDir()
		labels := taxonomy.Labels{Country: "vn", Release: "20990101"}
		for i := 1; i <= tc.count; i++ {
			labels.Articles = append(labels.Articles, taxonomy.LabelArticle{Key: "vn:vbpl:1", Anchor: fmt.Sprintf("dieu-%d", i), OwnTextSHA256: taxonomy.OwnTextSHA256("Synthetic"), Codes: []string{"1.1"}})
		}
		b, err := json.Marshal(labels)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "labels.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"vn", "20990101", "--families", filepath.Join(dir, "families.json"), "--select", "labels:" + filepath.Join(dir, "labels.json"), "--recipe", "claude-haiku-5-5", "--out", filepath.Join(dir, "output"), "--budget", "pilot", "--budget-usd", "15", "--dry-run"}
		loaded := false
		var out, errout bytes.Buffer
		code := runClassifyArticlesWith(context.Background(), args, &out, &errout, func(context.Context, string, string, taxonomy.Labels) (classificationEvidence, error) {
			loaded = true
			return classificationEvidence{}, errors.New("stop after count check")
		})
		if code != 1 || loaded != tc.loaded {
			t.Fatalf("count %d: code %d loaded %v stderr %s", tc.count, code, loaded, &errout)
		}
		if !tc.loaded && !strings.Contains(errout.String(), "at most 300 labeled articles") {
			t.Fatalf("count %d: stderr %s", tc.count, &errout)
		}
	}
}
