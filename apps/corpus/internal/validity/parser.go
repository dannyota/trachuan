// Package validity parses narrow forms of effect articles.
package validity

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Date is a civil date. Its zero value means no date was stated.
type Date struct{ Year, Month, Day int }

// Item describes a replacement or repeal named in a sentence.
type Item struct {
	Verb, Scope            string
	InstrumentType, Number string
	IssueDate              Date
	Issuer, Sentence, Text string
}

// Reason explains why an article cannot be used automatically.
type Reason struct{ Kind, Sentence string }

// Result holds the reading and any reasons it is unresolved.
type Result struct {
	Usable       bool
	Commencement Date
	Items        []Item
	Reasons      []Reason
}

// Select reports whether heading identifies an effect article.
func Select(heading string) bool {
	s := strings.ToLower(normalizeText(heading))
	return s == "hiệu lực thi hành" || s == "điều khoản thi hành"
}

// Parse reads the article of a document with the given Vietnamese type.
func Parse(documentType, article string) Result {
	var result Result
	lines := strings.SplitN(article, "\n", 2)
	if len(lines) < 2 {
		result.Reasons = append(result.Reasons, Reason{Kind: "missing_commencement"})
		return result
	}
	bodyLines := strings.Split(lines[1], "\n")
	for i := range bodyLines {
		bodyLines[i] = normalizeText(bodyLines[i])
	}
	body := regexp.MustCompile(`(?m)^\s*\d+\.\s*`).ReplaceAllString(strings.Join(bodyLines, "\n"), "")
	body = normalizeText(body)
	documentType = normalizeText(documentType)
	count := 0
	for _, raw := range splitSentences(body) {
		sentence := strings.TrimSpace(raw)
		if sentence == "" {
			continue
		}
		clean := stripQuotes(sentence)
		lower := strings.ToLower(norm.NFC.String(clean))
		if !hasEffect(lower) {
			continue
		}
		if strings.Contains(lower, "có hiệu lực") {
			date, exception, ok := parseCommencement(documentType, clean)
			if ok {
				count++
				if count == 1 {
					result.Commencement = date
				}
				if exception {
					result.Reasons = append(result.Reasons, Reason{"exception", sentence})
				}
				continue
			}
			if clauseScoped(lower) {
				result.Reasons = append(result.Reasons, Reason{"clause_scoped_commencement", sentence})
				continue
			}
		}
		items, ok := parseReplacement(documentType, clean, sentence)
		if ok {
			result.Items = append(result.Items, items...)
			continue
		}
		result.Reasons = append(result.Reasons, Reason{"unparsed_effect_sentence", sentence})
	}
	if count == 0 {
		result.Reasons = append(result.Reasons, Reason{Kind: "missing_commencement"})
	}
	if count > 1 {
		result.Reasons = append(result.Reasons, Reason{Kind: "multiple_commencement_sentences"})
	}
	result.Usable = len(result.Reasons) == 0
	return result
}

func hasEffect(s string) bool {
	for _, w := range []string{"có hiệu lực", "thay thế", "bãi bỏ", "hết hiệu lực"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
func clauseScoped(s string) bool {
	return regexp.MustCompile(`(?i)(?:^|\s)(khoản|điều|điểm|chương|mục|phụ lục)\s+[^.]*có hiệu lực`).MatchString(s)
}
func stripQuotes(s string) string {
	var b strings.Builder
	var close rune
	for _, r := range s {
		if close != 0 {
			if r == close {
				close = 0
			}
			b.WriteRune(' ')
			continue
		}
		switch r {
		case '"':
			close = '"'
		case '“':
			close = '”'
		case '‘':
			close = '’'
		case '«':
			close = '»'
		}
		if close != 0 {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitSentences(s string) []string {
	var sentences []string
	start := 0
	var close rune
	for i, r := range s {
		if close != 0 {
			if r == close {
				close = 0
			}
			continue
		}
		switch r {
		case '"':
			close = '"'
		case '“':
			close = '”'
		case '‘':
			close = '’'
		case '«':
			close = '»'
		case '.':
			sentences = append(sentences, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		sentences = append(sentences, s[start:])
	}
	return sentences
}

func normalizeText(s string) string {
	return strings.Join(strings.Fields(norm.NFC.String(s)), " ")
}
