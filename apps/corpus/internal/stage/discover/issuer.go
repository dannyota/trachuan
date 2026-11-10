package discover

import (
	"regexp"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/source"
)

func classifyIssuer(fields []chinhphu.FieldOccurrence, forms []string) string {
	value := ""
	found := false
	for _, field := range fields {
		if strings.TrimSpace(field.Name) != "Cơ quan ban hành" {
			continue
		}
		if field.Ambiguous {
			return "unknown"
		}
		current := source.NormalizeIssuer(field.Value)
		if current == "" || found && value != current {
			return "unknown"
		}
		value, found = current, true
	}
	if !found {
		return "unknown"
	}
	for _, form := range forms {
		if value == source.NormalizeIssuer(form) {
			return "central"
		}
	}
	if matched, _ := regexp.MatchString(`^(CHỦ TỊCH )?(UBND|ỦY BAN NHÂN DÂN|HĐND|HỘI ĐỒNG NHÂN DÂN) (TỈNH|THÀNH PHỐ) .+$`, value); matched {
		return "provincial"
	}
	return "unknown"
}
