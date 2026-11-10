package validity

import (
	"regexp"
	"strings"
)

func parseReplacement(documentType, clean, sentence string) ([]Item, bool) {
	if !strings.HasSuffix(strings.TrimSpace(clean), ".") {
		return nil, false
	}
	subject := `(?i)^\s*` + regexp.QuoteMeta(strings.TrimSpace(documentType)) + `\s+này\s+(thay\s+thế|bãi\s+bỏ)\s+`
	match := regexp.MustCompile(subject).FindStringSubmatchIndex(clean)
	if match == nil {
		return nil, false
	}
	verb := verbName(clean[match[2]:match[3]])
	rest := strings.TrimSpace(strings.TrimSuffix(clean[match[1]:], "."))
	if verb == "replaces" {
		rest = regexp.MustCompile(`(?i)^cho\s+`).ReplaceAllString(rest, "")
	}
	// Split only at an explicit new verb or an instrument start, not descriptive commas.
	split := regexp.MustCompile(`(?i)\s*;\s*|\s*,\s*(?:và\s+)?(?:` + instrumentStart() + `)|\s+và\s+(?:bãi\s+bỏ\s+|thay\s+thế\s+(?:cho\s+)?|` + instrumentStart() + `)`)
	positions := split.FindAllStringIndex(rest, -1)
	var chunks []string
	start := 0
	for _, p := range positions {
		chunks = append(chunks, strings.TrimSpace(rest[start:p[0]]))
		delimiter := rest[p[0]:p[1]]
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(delimiter), ","))
		trimmed = regexp.MustCompile(`(?i)^và\s+`).ReplaceAllString(trimmed, "")
		if trimmed != ";" && trimmed != "" {
			start = p[0] + strings.Index(delimiter, trimmed)
		} else {
			start = p[1]
		}
	}
	chunks = append(chunks, strings.TrimSpace(rest[start:]))
	var items []Item
	for _, chunk := range chunks {
		v := regexp.MustCompile(`(?i)^(thay\s+thế|bãi\s+bỏ)\s+(?:cho\s+)?`).FindStringSubmatchIndex(chunk)
		if v != nil {
			verb = verbName(chunk[v[2]:v[3]])
			chunk = chunk[v[1]:]
		}
		if hasEffect(strings.ToLower(chunk)) {
			return nil, false
		}
		item, ok := parseItem(chunk)
		if !ok {
			return nil, false
		}
		item.Text = strings.TrimSpace(chunk)
		item.Verb = verb
		item.Sentence = sentence
		items = append(items, item)
	}
	return items, len(items) > 0
}

func instrumentStart() string {
	return `(?:nghị\s+định|luật|thông\s+tư|quyết\s+định|nghị\s+quyết|pháp\s+lệnh|điều|khoản|điểm|chương|mục|phụ\s+lục|cụm\s+từ)\s+`
}
func verbName(s string) string {
	if strings.HasPrefix(strings.ToLower(s), "thay") {
		return "replaces"
	}
	return "repeals"
}
func parseItem(s string) (Item, bool) {
	if regexp.MustCompile(`(?i)(?:^|[^\pL\pN_])trừ(?:$|[^\pL\pN_])|không bao gồm`).MatchString(s) {
		return Item{}, false
	}
	partial := regexp.MustCompile(`(?i)^(?:điều|khoản|điểm|chương|mục|phụ\s+lục|cụm\s+từ)\s+\S`)
	if partial.MatchString(s) {
		return Item{Scope: "partial"}, true
	}
	pattern := `(?i)^(nghị\s+định|luật|thông\s+tư|quyết\s+định|nghị\s+quyết|pháp\s+lệnh)\s+số\s+(\S+)\s+ngày\s+(` + datePattern + `)(?:\s+của\s+(.+)|\s+.+)?$`
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if m == nil {
		return Item{}, false
	}
	item := Item{Scope: "whole", InstrumentType: m[1], Number: m[2], Issuer: strings.TrimSpace(m[4])}
	if m[3] != "" {
		var ok bool
		item.IssueDate, ok = parseDate(m[3])
		if !ok {
			return Item{}, false
		}
	}
	return item, true
}
