package textrule

import "strings"

// Unit is one source-ordered text unit. FirstItem marks a direct clause's first
// sentence; only Kind dieu admits numbered line items. Close requires closed
// quotation state after this unit.
type Unit struct {
	Text      string
	Kind      string
	FirstItem bool
	Close     bool
}

type reading struct {
	text       string
	mask       []bool
	ends       map[int]bool
	firstItems map[int]bool
	articles   map[int]bool
}

func normalizeLines(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n")
	for i := range lines {
		lines[i] = normalize(lines[i])
	}
	return strings.Join(lines, "\n")
}

func readUnits(units []Unit) (reading, bool) {
	r := reading{ends: map[int]bool{}, firstItems: map[int]bool{}, articles: map[int]bool{}}
	var parts []string
	for _, unit := range units {
		part := normalizeLines(unit.Text)
		start := len(r.text)
		r.text += part + "\n"
		r.ends[len(r.text)-1] = true
		r.articles[start] = unit.Kind == "dieu"
		if unit.FirstItem {
			r.firstItems[start] = true
		}
		parts = append(parts, part)
		if unit.Close {
			mask, balanced := quoteMask(strings.Join(parts, "\n") + "\n")
			if !balanced {
				return r, false
			}
			r.mask = append(r.mask, mask...)
			parts = nil
		}
	}
	if len(parts) > 0 {
		mask, balanced := quoteMask(strings.Join(parts, "\n") + "\n")
		if !balanced {
			return r, false
		}
		r.mask = append(r.mask, mask...)
	}
	return r, true
}

func lineMarker(text string) bool {
	text, _, _ = strings.Cut(text, "\n")
	c := cursor{text: text, tokens: tokenize(text, make([]bool, len(text)))}
	c.listMarker()
	return c.pos != 0
}

func numericMarker(c cursor) bool {
	return len(c.tokens) >= 2 && digits(c.tokens[0].word) && c.tokens[1].word == "." && !c.tokens[0].quoted
}

func wordingEnd(text string) bool {
	text = strings.TrimSpace(strings.TrimRight(strings.ToLower(text), ":"))
	return strings.HasSuffix(text, "như sau")
}

func sentencePunctuation(text string) bool {
	return strings.ContainsAny(text[len(text)-1:], ".;:")
}

func articleLabel(text string) bool {
	c := cursor{text: text, tokens: tokenize(text, make([]bool, len(text)))}
	return c.take("điều") && c.pos < len(c.tokens) && numberLabel(c.tokens[c.pos].word) && c.pos+1 == len(c.tokens)
}

func (r reading) scan() []cursor {
	var out []cursor
	start := 0
	lineStart, firstItem := true, r.firstItems[0]
	articleUnit := r.articles[0]
	add := func(end int, broken bool) {
		part := strings.TrimSpace(r.text[start:end])
		if part != "" {
			c := readingCursor(part)
			c.top = firstItem || (articleUnit && lineStart && numericMarker(c))
			c.broken = broken
			out = append(out, c)
			firstItem = false
		}
		start = end
	}
	for i := 0; i < len(r.text); i++ {
		if r.mask[i] {
			// Phrase quotations stay in their sentence. New wording and standalone
			// quotations form their own unit and cannot expose inner candidates.
			if i == start || i == 0 || !r.mask[i-1] {
				prefix := strings.TrimSpace(r.text[start:i])
				if prefix == "" || wordingEnd(prefix) {
					add(i, false)
					end := quotationEnd(r.text, i)
					add(end, false)
					i = end - 1
					lineStart = false
				}
			}
			continue
		}
		if r.text[i] == '\n' {
			next := strings.TrimLeft(r.text[i+1:], " \n")
			boundary := r.ends[i] || lineMarker(next) || startsQuote(next)
			if boundary {
				part := strings.TrimSpace(r.text[start:i])
				add(i, !r.ends[i] && part != "" && !sentencePunctuation(part))
				start = i + 1
				lineStart = true
				if r.ends[i] {
					firstItem = r.firstItems[i+1]
					articleUnit = r.articles[i+1]
				}
			}
			continue
		}
		if r.text[i] == '.' && (i+1 == len(r.text) || r.text[i+1] == ' ' || r.text[i+1] == '\n') {
			part := strings.TrimSpace(r.text[start:i])
			if digits(part) || articleLabel(part) {
				continue
			}
			add(i+1, false)
			lineStart = false
		}
	}
	add(len(r.text), false)
	for i := range out {
		c := readingCursor(normalize(out[i].text))
		out[i].text, out[i].tokens = c.text, c.tokens
	}
	return out
}

func startsQuote(text string) bool {
	return strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") || strings.HasPrefix(text, "“") || strings.HasPrefix(text, "«")
}

func quotedSentence(c cursor) bool {
	return len(c.tokens) > 0 && c.tokens[0].quoted
}

func punctuationOnly(c cursor) bool {
	return strings.Trim(c.text, ".;: ") == ""
}

func validWording(parts []cursor, i int) bool {
	if i+1 >= len(parts) || !quotedSentence(parts[i+1]) {
		return false
	}
	i += 2
	for i < len(parts) && punctuationOnly(parts[i]) {
		i++
	}
	return i == len(parts) || !quotedSentence(parts[i])
}

func introducesWording(c cursor) bool {
	for c.pos < len(c.tokens) {
		if c.take("như", "sau") {
			return true
		}
		c.pos++
	}
	return false
}

func readingCursor(text string) cursor {
	mask, balanced := quoteMask(text)
	if !balanced {
		mask = make([]bool, len(text))
	}
	return cursor{text: text, tokens: tokenize(text, mask)}
}
