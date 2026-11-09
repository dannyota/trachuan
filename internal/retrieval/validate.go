package retrieval

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Validation contains the input gate and document-number routing result.
type Validation struct {
	Accepted   bool
	Routed     bool
	References []string
	Reason     string
	Message    string
}

// ValidateQuery applies the deterministic query gate.
func ValidateQuery(q string) (Validation, error) {
	if !utf8.ValidString(q) || utf8.RuneCountInString(q) > 2000 || strings.TrimSpace(q) == "" || strings.ContainsRune(q, 0) {
		return Validation{}, errors.New("invalid_request")
	}
	refs, err := ParseReferences(norm.NFC.String(q))
	if err != nil {
		return Validation{}, errors.New("invalid_request")
	}
	v := Validation{References: refs, Routed: len(refs) > 0}
	var latin, marked bool
	for _, r := range norm.NFC.String(q) {
		if unicode.In(r, unicode.Latin) && unicode.IsLetter(r) {
			latin = true
		}
		if r == 'đ' || r == 'Đ' {
			marked = true
			continue
		}
		d := []rune(norm.NFD.String(string(r)))
		if len(d) < 2 || !strings.ContainsRune("aeiouyAEIOUY", d[0]) {
			continue
		}
		for _, m := range d[1:] {
			if strings.ContainsRune("\u0306\u0302\u031b\u0301\u0300\u0309\u0303\u0323", m) {
				marked = true
			}
		}
	}
	if v.Routed || marked {
		v.Accepted = true
		return v, nil
	}
	v.Reason = "diacritics_required"
	if !latin {
		v.Reason = "unsupported_query"
	}
	v.Message = "Vui lòng nhập câu hỏi tiếng Việt có dấu hoặc số hiệu văn bản."
	return v, nil
}
