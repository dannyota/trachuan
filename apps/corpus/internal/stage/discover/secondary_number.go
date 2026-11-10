package discover

import (
	"strings"
	"unicode"

	"danny.vn/trachuan/internal/retrieval"

	"golang.org/x/text/unicode/norm"
)

func secondaryWireNumber(s string) string {
	s = strings.ToUpper(norm.NFC.String(strings.TrimSpace(s)))
	s = strings.NewReplacer("А", "A", "В", "B", "С", "C", "Е", "E", "Н", "H", "К", "K", "М", "M", "О", "O", "Р", "P", "Т", "T", "Х", "X").Replace(s)
	var out strings.Builder
	rs := []rune(s)
	for i, c := range rs {
		if unicode.IsSpace(c) {
			a, b := i-1, i+1
			for a >= 0 && unicode.IsSpace(rs[a]) {
				a--
			}
			for b < len(rs) && unicode.IsSpace(rs[b]) {
				b++
			}
			if a >= 0 && (rs[a] == '/' || rs[a] == '-') || b < len(rs) && (rs[b] == '/' || rs[b] == '-') {
				continue
			}
		}
		out.WriteRune(c)
	}
	return out.String()
}

func knownListingNumber(s string) bool {
	found, err := retrieval.ParseReferences(s)
	return err == nil && len(found) == 1 && found[0] == retrieval.NormalizeNumber(s)
}

func secondarySearchText(d secondaryDefinition) string {
	if d.Kind == "named" {
		return secondaryWireNumber(d.Number)
	}
	return d.Keyword
}
