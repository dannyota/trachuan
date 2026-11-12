package textrule

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

type token struct {
	word       string
	start, end int
	quoted     bool
}

func normalize(text string) string {
	var out strings.Builder
	for _, r := range norm.NFC.String(text) {
		out.WriteRune(asciiDigit(r))
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func asciiDigit(r rune) rune {
	if !unicode.IsDigit(r) {
		return r
	}
	for _, entry := range unicode.Digit.R16 {
		if uint32(r) >= uint32(entry.Lo) && uint32(r) <= uint32(entry.Hi) && (uint32(r)-uint32(entry.Lo))%uint32(entry.Stride) == 0 {
			return '0' + rune((uint32(r)-uint32(entry.Lo))/uint32(entry.Stride)%10)
		}
	}
	for _, entry := range unicode.Digit.R32 {
		if uint32(r) >= entry.Lo && uint32(r) <= entry.Hi && (uint32(r)-entry.Lo)%entry.Stride == 0 {
			return '0' + rune((uint32(r)-entry.Lo)/entry.Stride%10)
		}
	}
	return r
}

func tokenize(text string, quoted []bool) []token {
	var tokens []token
	for i := 0; i < len(text); {
		r, width := utf8.DecodeRuneInString(text[i:])
		if unicode.IsSpace(r) {
			i += width
			continue
		}
		start := i
		i += width
		if wordRune(r) {
			for i < len(text) {
				r, width = utf8.DecodeRuneInString(text[i:])
				if !wordRune(r) {
					break
				}
				i += width
			}
		}
		tokens = append(tokens, token{word: strings.ToLower(text[start:i]), start: start, end: i, quoted: quoted[start]})
	}
	return tokens
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func letter(s string) bool {
	r, width := utf8.DecodeRuneInString(s)
	return width == len(s) && unicode.IsLetter(r)
}

type cursor struct {
	text   string
	tokens []token
	pos    int
}

func (c *cursor) at(word string) bool {
	return c.pos < len(c.tokens) && !c.tokens[c.pos].quoted && c.tokens[c.pos].word == word
}

func (c *cursor) take(words ...string) bool {
	start := c.pos
	for _, word := range words {
		if !c.at(word) {
			c.pos = start
			return false
		}
		c.pos++
	}
	return true
}

func (c *cursor) raw(start, end int) string {
	return c.text[c.tokens[start].start:c.tokens[end-1].end]
}

func (c *cursor) listMarker() {
	if c.pos+1 >= len(c.tokens) || c.tokens[c.pos].quoted {
		return
	}
	word := c.tokens[c.pos].word
	if (digits(word) && c.tokens[c.pos+1].word == ".") || (letter(word) && c.tokens[c.pos+1].word == ")") {
		c.pos += 2
	}
}
