package build

import "fmt"

type gapRule struct {
	Class    string `json:"class"`
	Blocking string `json:"blocking"`
}

func gapKinds() map[string]gapRule {
	return map[string]gapRule{
		"mapping_unresolved": {"public", "never"}, "mapping_dropped": {"public", "never"},
		"title_missing": {"public", "licensed"}, "ocr_blocked": {"text", "always"},
		"ocr_flagged": {"text", "never"}, "text_incomplete": {"text", "never"},
		"chunk_omission": {"text", "never"}, "embedding_input_too_long": {"text", "never"},
		"embedding_input_bytes_exceeded": {"text", "never"},
	}
}

func classifyGap(kind, access string) (string, bool, error) {
	rule, ok := gapKinds()[kind]
	if !ok {
		return "", false, fmt.Errorf("unknown gap kind")
	}
	if access != "free" && access != "licensed" {
		return "", false, fmt.Errorf("unknown gap access")
	}
	return rule.Class, rule.Blocking == "always" || rule.Blocking == "licensed" && access == "licensed", nil
}
