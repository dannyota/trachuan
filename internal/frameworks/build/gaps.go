package build

import (
	"fmt"
	"sort"
)

func writeGaps(dir string, gaps []Gap) error {
	for _, gap := range gaps {
		switch gap.Kind {
		case "mapping_unresolved", "mapping_dropped", "title_missing", "ocr_blocked", "ocr_flagged", "text_incomplete", "chunk_omission", "embedding_input_too_long", "embedding_input_bytes_exceeded":
		default:
			return fmt.Errorf("unknown gap kind")
		}
		if gap.Count <= 0 {
			return fmt.Errorf("invalid gap count")
		}
	}
	sort.SliceStable(gaps, func(i, j int) bool {
		a, b := gaps[i], gaps[j]
		ac, bc := "", ""
		if a.Citation != nil {
			ac = *a.Citation
		}
		if b.Citation != nil {
			bc = *b.Citation
		}
		for _, pair := range [][2]string{{a.Kind, b.Kind}, {a.Code, b.Code}, {a.Version, b.Version}, {ac, bc}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return a.Count < b.Count
	})
	return writeRows(dir, "gaps.jsonl", gaps)
}
