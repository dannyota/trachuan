package fetch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func resolveChinhphu(dir string, item stage.Item) (stage.Resolution, error) {
	doc, _, err := readSecondary(dir)
	if err != nil {
		return stage.Resolution{}, err
	}
	if doc.Key != item.Key {
		return stage.Resolution{}, fmt.Errorf("fetch identity disagrees with planned key %q", item.Key)
	}
	b, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("read report: %w", err)
	}
	var report secondaryReport
	if err = json.Unmarshal(b, &report); err != nil {
		return stage.Resolution{}, fmt.Errorf("decode report: %w", err)
	}
	if report.Version != 1 {
		return stage.Resolution{}, fmt.Errorf("unknown report version")
	}
	gaps := []stage.Gap{}
	for _, v := range report.Missing {
		gaps = append(gaps, stage.Gap{Kind: "missing_attachment", Detail: v.Name})
	}
	for _, v := range report.TooLarge {
		gaps = append(gaps, stage.Gap{Kind: "file_too_large", Detail: v.Name})
	}
	return stage.Resolution{Gaps: gaps}, nil
}
