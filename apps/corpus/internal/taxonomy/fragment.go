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
	document := `(?:thông tư|nghị định|quyết định|luật|văn bản|nghị quyết|chỉ thị)`
	part := `(?:điều|khoản|điểm|phụ lục|mẫu)`
	number := `[0-9]+(?:/[0-9]+)*(?:/[a-zđ]+(?:-[a-zđ]+)*)?`
	referenceWord := `(?:điều|khoản|điểm|phụ|lục|mẫu|thông|tư|nghị|định|quyết|luật|văn|bản|chỉ|thị|này|của|số|ngày|tháng|năm|ban hành kèm theo|đính kèm|và|bằng|` + number + `)`
	reference := `(?:` + part + `|` + document + `)(?:[ ,]+` + referenceWord + `)*`
	date := `[0-9]+(?:/[0-9]+/[0-9]+|\s+tháng\s+[0-9]+\s+năm\s+[0-9]+)`
	subject := `(?:` + document + `\s+này|` + part + `\s+(?:này|[0-9]+)(?:\s+điều\s+[0-9]+)?)`
	addressee := `(?:bộ trưởng|thủ trưởng(?: các đơn vị thuộc ngân hàng nhà nước(?: việt nam)?| cơ quan ngang bộ)?|chánh văn phòng|vụ trưởng vụ chính sách tiền tệ|chủ tịch ủy ban nhân dân các tỉnh|thành phố trực thuộc trung ương|tổ chức tài chính vi mô|tổ chức tín dụng|tổ chức|cá nhân(?: có liên quan)?|chi nhánh ngân hàng nước ngoài|ngân hàng|cơ quan)`
	addresseeList := `(?:các\s+)?` + addressee + `(?:(?:,\s*|\s+và\s+)(?:và\s+)?(?:các\s+)?` + addressee + `)*`

	patterns := []string{
		`^` + subject + `\s+(?:có hiệu lực(?: thi hành)?|hết hiệu lực(?: thi hành)?)(?: kể)? từ ngày ` + date + `$`,
		`^` + addresseeList + `\s+(?:chịu trách nhiệm (?:tổ chức thực hiện|thi hành|hướng dẫn thi hành|triển khai thực hiện)|hướng dẫn thi hành)\s+` + document + `\s+này$`,
		`^bãi bỏ\s+` + reference + `$`,
		`^thay thế\s+` + reference + `\s+bằng\s+` + reference + `$`,
		`^` + reference + `\s+được sửa đổi, bổ sung như sau:$`,
	}
	compiled := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		compiled[i] = regexp.MustCompile(p)
	}
	var sentences []string
	for _, line := range strings.Split(text, "\n") {
		line = regexp.MustCompile(`^\s*\d+\.\s*`).ReplaceAllString(line, "")
		for _, sentence := range splitFragmentSentences(line) {
			sentence = strings.TrimSpace(strings.Trim(sentence, " ./\t\r"))
			if sentence != "" && !regexp.MustCompile(`^\d+$`).MatchString(sentence) {
				sentences = append(sentences, sentence)
			}
		}
	}
	for i, sentence := range sentences {
		matched := false
		for j, p := range compiled {
			if p.MatchString(sentence) && (j != len(compiled)-1 || i == len(sentences)-1) {
				matched = true
				break
			}
		}
		if !matched {
			return false
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
