package taxonomy

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func IsFragment(text string) bool {
	text = strings.ToLower(norm.NFC.String(text))
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && regexp.MustCompile(`^\s*điều\s+\d+[a-zđ]?\.`).MatchString(lines[0]) {
		lines = lines[1:]
	}
	text = strings.Join(lines, "\n")
	patterns := []string{
		`^(thông tư|nghị định|quyết định|luật|văn bản|nghị quyết|chỉ thị|điều|khoản|phụ lục)[^:;]*\b(có hiệu lực thi hành từ|có hiệu lực kể từ|hết hiệu lực)(\s+(thi hành|kể từ|từ))?\s+(ngày|tháng|năm)\s+[0-9/ -]+(tháng\s+[0-9]+\s+năm\s+[0-9]+)?$`,
		`^[^:;]*\b(chịu trách nhiệm tổ chức thực hiện|chịu trách nhiệm thi hành|hướng dẫn thi hành)\s+(thông tư|nghị định|quyết định|luật|văn bản|nghị quyết|chỉ thị|quy định)\s+này$`,
		`^bãi bỏ\s+[^:;]+$`,
		`^thay thế\s+phụ lục\s+[^:;]+\s+bằng\s+phụ lục\s+[^:;]+$`,
		`^[^:;]+được sửa đổi, bổ sung như sau:$`,
	}
	compiled := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		compiled[i] = regexp.MustCompile(p)
	}
	for _, line := range strings.Split(text, "\n") {
		line = regexp.MustCompile(`^\s*\d+\.\s*`).ReplaceAllString(line, "")
		for _, sentence := range splitFragmentSentences(line) {
			sentence = strings.TrimSpace(strings.Trim(sentence, " ./\t\r"))
			if sentence == "" || regexp.MustCompile(`^\d+$`).MatchString(sentence) {
				continue
			}
			matched := false
			for _, p := range compiled {
				if p.MatchString(sentence) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}

func splitFragmentSentences(s string) []string {
	r := []rune(s)
	start := 0
	var out []string
	for i, c := range r {
		if c != '.' && c != ';' && c != '!' && c != '?' {
			continue
		}
		if c == '.' && i > 0 && i+1 < len(r) && unicode.IsDigit(r[i-1]) && unicode.IsDigit(r[i+1]) {
			continue
		}
		out = append(out, string(r[start:i]))
		start = i + 1
	}
	return append(out, string(r[start:]))
}
