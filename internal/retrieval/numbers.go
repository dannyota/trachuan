package retrieval

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// NumberNormalizationVersion identifies the document number matching rules.
const NumberNormalizationVersion = "document-number/2"

// NormalizeNumber applies the corpus document-number normalization.
func NormalizeNumber(s string) string {
	s = strings.ToUpper(norm.NFC.String(strings.TrimSpace(s)))
	s = strings.NewReplacer("Đ", "D", "А", "A", "В", "B", "Е", "E", "К", "K", "М", "M", "Н", "H", "О", "O", "Р", "P", "С", "C", "Т", "T", "Х", "X").Replace(s)
	var b strings.Builder
	r := []rune(s)
	for i, c := range r {
		if unicode.IsSpace(c) {
			j := i - 1
			for j >= 0 && unicode.IsSpace(r[j]) {
				j--
			}
			k := i + 1
			for k < len(r) && unicode.IsSpace(r[k]) {
				k++
			}
			if (j >= 0 && (r[j] == '/' || r[j] == '-')) || (k < len(r) && (r[k] == '/' || r[k] == '-')) {
				continue
			}
		}
		b.WriteRune(c)
	}
	return b.String()
}

// ParseReferences extracts bounded document-number references.
func ParseReferences(q string) ([]string, error) {
	if !utf8.ValidString(q) {
		return nil, errors.New("invalid UTF-8")
	}
	re := regexp.MustCompile(`[0-9]{1,4}[\s\p{Z}]*/[\s\p{Z}]*(?:[0-9]{4}[\s\p{Z}]*/[\s\p{Z}]*)?[\p{Latin}][\p{Latin}0-9]*(?:[\s\p{Z}]*-[\s\p{Z}]*[\p{Latin}0-9]+)*`)
	var out []string
	for _, loc := range re.FindAllStringIndex(q, -1) {
		if loc[0] > 0 {
			c, _ := utf8.DecodeLastRuneInString(q[:loc[0]])
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				continue
			}
		}
		if loc[1] < len(q) {
			c, _ := utf8.DecodeRuneInString(q[loc[1]:])
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				continue
			}
		}
		s := q[loc[0]:loc[1]]
		if utf8.RuneCountInString(s) > 128 || len(out) >= 8 {
			return nil, errors.New("document reference limit exceeded")
		}
		out = append(out, NormalizeNumber(s))
	}
	return out, nil
}
