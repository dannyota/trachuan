package textparse

import "strings"

// quotedParagraphs marks paragraphs in quoted text and returns the unclosed start.
func quotedParagraphs(p []Paragraph) ([]bool, int) {
	quoted := make([]bool, len(p))
	start := -1
	for i, para := range p {
		s := strings.TrimSpace(para.Text)
		runes := []rune(s)
		if start < 0 && len(runes) > 0 && strings.ContainsRune("“‘«\"", runes[0]) {
			start = i
		}
		if start < 0 {
			continue
		}
		quoted[i] = true
		end := strings.TrimSpace(strings.TrimRight(s, ".;,"))
		if (i > start || len([]rune(end)) > 1) && strings.ContainsAny(lastRune(end), "”’»\"") {
			start = -1
		}
	}
	return quoted, start
}

func lastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	return string(r[len(r)-1])
}
