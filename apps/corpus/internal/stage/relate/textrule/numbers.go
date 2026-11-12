package textrule

import (
	"strings"
	"unicode"

	"danny.vn/trachuan/internal/retrieval"
)

func numberWord(word string) bool {
	if word == "" {
		return false
	}
	for _, r := range word {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// number consumes a complete slash/hyphen token, retaining its raw spelling.
func (c *cursor) number() (string, bool) {
	start := c.pos
	i := start
	if i >= len(c.tokens) || !digits(c.tokens[i].word) {
		return "", false
	}
	i++
	// Dots within a serial are not sentence boundaries or terminal punctuation.
	for i+1 < len(c.tokens) && c.tokens[i].word == "." && digits(c.tokens[i+1].word) && c.tokens[i-1].end == c.tokens[i].start && c.tokens[i].end == c.tokens[i+1].start {
		i += 2
	}
	if i >= len(c.tokens) || c.tokens[i].word != "/" {
		return "", false
	}
	for i < len(c.tokens) && (c.tokens[i].word == "/" || c.tokens[i].word == "-") {
		i++
		if i >= len(c.tokens) || !numberWord(c.tokens[i].word) {
			return "", false
		}
		i++
	}
	raw := c.raw(start, i)
	parts := strings.Split(retrieval.NormalizeNumber(raw), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", false
	}
	suffix := parts[len(parts)-1]
	if suffix == "" || !unicode.IsLetter([]rune(suffix)[0]) {
		return "", false
	}
	c.pos = i
	return raw, true
}

func (c *cursor) documentType() string {
	switch {
	case c.take("thông", "tư", "liên", "tịch"):
		return "TTLT"
	case c.take("thông", "tư"):
		return "TT"
	case c.take("nghị", "định"):
		return "ND"
	case c.take("quyết", "định"):
		return "QD"
	case c.take("nghị", "quyết"):
		return "NQ"
	case c.take("luật"):
		return "QH"
	case c.take("pháp", "lệnh"):
		return "UBTVQH"
	default:
		return ""
	}
}

func checkNumber(raw, docType string, issuers map[string]string) (issuer, gap string) {
	parts := strings.Split(retrieval.NormalizeNumber(raw), "/")
	if len(parts) != 3 || len(parts[1]) != 4 || !digits(parts[1]) {
		return "", "relation_text_yearless_number"
	}
	suffix := strings.SplitN(parts[2], "-", 2)
	if len(suffix) == 1 {
		code := legislatureCode(suffix[0])
		if code == "QH" || code == "UBTVQH" {
			// Legislature suffixes carry both the document type and issuer.
			suffix = []string{code, code}
		}
	}
	if suffix[0] != docType {
		return "", "relation_text_type_mismatch"
	}
	if len(suffix) != 2 {
		return "", "relation_text_local_number"
	}
	issuerSegment := legislatureCode(suffix[1])
	for key := range issuers {
		if retrieval.NormalizeNumber(key) != issuerSegment {
			continue
		}
		// A malformed caller table must not make the result depend on map order.
		if issuer != "" {
			return "", "relation_text_local_number"
		}
		issuer = key
	}
	if issuer == "" {
		return "", "relation_text_local_number"
	}
	return issuer, ""
}

func legislatureCode(segment string) string {
	code := strings.TrimRight(segment, "0123456789")
	if code == "QH" || code == "UBTVQH" {
		return code
	}
	return segment
}
