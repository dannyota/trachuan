package extract

import (
	"regexp"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func numberTokens(s string) []string {
	s = norm.NFC.String(s)
	re := regexp.MustCompile(`[0-9][\pL\pN.]* ?[/\-] ?[\pL\pN.]+(?: ?[/\-] ?[\pL\pN.]+)*`)
	var tokens []string
	for _, loc := range re.FindAllStringIndex(s, -1) {
		if loc[0] > 0 {
			r, _ := utf8.DecodeLastRuneInString(s[:loc[0]])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
		}
		if loc[1] < len(s) {
			r, _ := utf8.DecodeRuneInString(s[loc[1]:])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
		}
		tokens = append(tokens, s[loc[0]:loc[1]])
	}
	return tokens
}

func headerPrefix(header string) string {
	count := 0
	for i := range header {
		if count == 4000 {
			return header[:i]
		}
		count++
	}
	return header
}

func headerNumbers(header string) []string {
	header = norm.NFC.String(headerPrefix(header))
	marker := regexp.MustCompile(`(?:Số[[:space:]]*:|SỐ[[:space:]]*:?)[[:space:]]*`)
	number := regexp.MustCompile(`^[0-9][\pL\pN.]* ?[/\-] ?[\pL\pN.]+(?: ?[/\-] ?[\pL\pN.]+)*`)
	var tokens []string
	pos := 0
	for count := 0; count < 50 && pos < len(header); count++ {
		loc := marker.FindStringIndex(header[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		pos = end
		if start > 0 {
			r, _ := utf8.DecodeLastRuneInString(header[:start])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
		}
		loc = number.FindStringIndex(header[end:])
		if loc == nil {
			continue
		}
		tokenEnd := end + loc[1]
		if tokenEnd < len(header) {
			r, _ := utf8.DecodeRuneInString(header[tokenEnd:])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
		}
		tokens = append(tokens, header[end:tokenEnd])
		pos = tokenEnd
	}
	return tokens
}

func titleHasNumber(title, token string) bool {
	normalized := normalizeNumber(token)
	for _, candidate := range numberTokens(title) {
		if normalizeNumber(candidate) == normalized {
			return true
		}
	}
	return false
}
