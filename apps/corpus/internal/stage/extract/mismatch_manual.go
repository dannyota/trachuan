package extract

import "sort"

// Reviewed records that hold another document's text, with evidence and check date.
const manualMismatch1200Key = "vn:vbpl:1200"
const manualMismatch1200Detail = "holds Sắc lệnh 14/SL of 1951-05-01; VBPL record error; checked 2026-11-12"

func manualMismatches() map[string]string {
	return map[string]string{manualMismatch1200Key: manualMismatch1200Detail}
}

func manualMismatchRecipe() []string {
	entries := manualMismatches()
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, 2*len(keys))
	for _, key := range keys {
		parts = append(parts, key, entries[key])
	}
	return parts
}
