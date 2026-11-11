package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func readTextMetadata(dir string) (string, map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "text.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "source_tree", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("read text record: %w", err)
	}
	var r struct {
		TextSource string `json:"text_source"`
		Provisions []struct {
			NodeID                   string
			StartOrdinal, EndOrdinal int
		} `json:"provisions"`
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return "", nil, fmt.Errorf("decode text record: %w", err)
	}
	locators := make(map[string]string, len(r.Provisions))
	for _, p := range r.Provisions {
		locators[p.NodeID] = fmt.Sprintf("block:%d-%d", p.StartOrdinal, p.EndOrdinal)
	}
	return r.TextSource, locators, nil
}
func textComplete(n int, key string, gaps []InventoryGap) bool {
	if n == 0 {
		return false
	}
	for _, g := range gaps {
		if g.Stage != "extract" || g.ItemKey != key {
			continue
		}
		switch g.Kind {
		case "structure_uncertain", "table_text", "image_content", "ocr_flagged", "needs_ocr", "text_limit", "no_text":
			return false
		}
	}
	return true
}
