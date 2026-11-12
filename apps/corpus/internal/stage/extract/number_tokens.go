package extract

import (
	"regexp"
	"strings"
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

func headerNumbers(header string) []string {
	header = norm.NFC.String(header)
	re := regexp.MustCompile(`(?:Số[[:space:]]*:|SỐ[[:space:]]*:?)[[:space:]]*`)
	var tokens []string
	for _, loc := range re.FindAllStringIndex(header, -1) {
		if loc[0] > 0 {
			r, _ := utf8.DecodeLastRuneInString(header[:loc[0]])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
		}
		rest := header[loc[1]:]
		found := numberTokens(rest)
		if len(found) > 0 && strings.HasPrefix(rest, found[0]) {
			tokens = append(tokens, found[0])
		}
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
