package extract

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func subjectMismatch(header, title string) bool {
	types := `THÔNG TƯ LIÊN TỊCH|THÔNG TƯ|QUYẾT ĐỊNH|NGHỊ ĐỊNH|CHỈ THỊ|SẮC LỆNH|LUẬT|NGHỊ QUYẾT|PHÁP LỆNH`
	re := regexp.MustCompile(`(?i)(` + types + `)`)
	subject := ""
	for _, loc := range re.FindAllStringIndex(header, -1) {
		if loc[0] > 0 {
			r, _ := utf8.DecodeLastRuneInString(header[:loc[0]])
			if unicode.IsLetter(r) {
				continue
			}
		}
		if loc[1] < len(header) {
			r, _ := utf8.DecodeRuneInString(header[loc[1]:])
			if unicode.IsLetter(r) {
				continue
			}
		}
		rest := strings.TrimSpace(header[loc[1]:])
		words := strings.Fields(rest)
		if len(words) == 0 || strings.EqualFold(words[0], "CỦA") || strings.EqualFold(words[0], "SỐ") {
			continue
		}
		subject = rest
		break
	}
	if utf8.RuneCountInString(subject) < 10 {
		return false
	}
	prefix := regexp.MustCompile(`(?i)^\s*(?:` + types + `)\s+số\s*:?\s*\S+\s*`)
	title = prefix.ReplaceAllString(title, "")
	a, b := significantWords(subject), significantWords(title)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	common := 0
	for word := range a {
		if b[word] {
			common++
		}
	}
	size := len(a)
	if len(b) < size {
		size = len(b)
	}
	return 2*common < size
}

func significantWords(s string) map[string]bool {
	s = strings.ToLower(norm.NFC.String(s))
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	out := make(map[string]bool)
	for _, word := range words {
		if utf8.RuneCountInString(word) < 2 {
			continue
		}
		if strings.IndexFunc(word, func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
			continue
		}
		out[word] = true
	}
	return out
}
