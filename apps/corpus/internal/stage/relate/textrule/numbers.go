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

// number consumes a complete number candidate, retaining its raw spelling.
func (c *cursor) number() (string, bool) {
	raw, end, ok := c.numberCandidate()
	if ok {
		c.pos = end
	}
	return raw, ok
}

// numberCandidate returns the whole candidate's end even when its shape fails.
func (c *cursor) numberCandidate() (string, int, bool) {
	start := c.pos
	if start >= len(c.tokens) || !digits(c.tokens[start].word) {
		return "", start + 1, false
	}
	end := c.numberEnd(start)
	raw := c.raw(start, end)
	parts := strings.Split(retrieval.NormalizeNumber(raw), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", end, false
	}
	suffix := parts[len(parts)-1]
	if suffix == "" || !unicode.IsLetter([]rune(suffix)[0]) {
		return "", end, false
	}
	for _, segment := range strings.Split(suffix, "-") {
		if segment == "" {
			return "", end, false
		}
	}
	return raw, end, true
}

func (c *cursor) numberEnd(start int) int {
	end := start + 1
	for end < len(c.tokens) {
		previous, next := c.tokens[end-1], c.tokens[end]
		switch next.word {
		case ",", ";", ")":
			return end
		case ".":
			if next.end == len(c.text) || c.text[next.end] == ' ' {
				return end
			}
		}
		// Official number normalization allows whitespace around these separators.
		if previous.end < next.start && previous.word != "/" && previous.word != "-" && next.word != "/" && next.word != "-" {
			return end
		}
		end++
	}
	return end
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
	for _, segment := range strings.Split(parts[0], ".") {
		if !digits(segment) {
			return "", "relation_text_local_number"
		}
	}
	for _, segment := range strings.Split(parts[2], "-") {
		if !numberWord(segment) {
			return "", "relation_text_local_number"
		}
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
