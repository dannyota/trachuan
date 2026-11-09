package lexical

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Tokenize folds and splits text according to the recipe.
func Tokenize(text string, recipe Recipe) ([]string, error) {
	if err := recipe.validate(); err != nil {
		return nil, err
	}
	text = norm.NFD.String(strings.ToLower(text))
	var b strings.Builder
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'đ':
			b.WriteByte('d')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String()), nil
}
