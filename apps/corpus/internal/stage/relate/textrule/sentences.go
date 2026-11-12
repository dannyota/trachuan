package textrule

import "strings"

// quoteMask marks quoted bytes, including delimiters, across the whole provision.
func quoteMask(text string) ([]bool, bool) {
	mask := make([]bool, len(text))
	var stack []rune
	for i, r := range text {
		inside := len(stack) > 0
		switch r {
		case '"', '\'':
			if inside && stack[len(stack)-1] == r {
				stack = stack[:len(stack)-1]
			} else {
				stack = append(stack, r)
			}
			inside = true
		case '“':
			stack = append(stack, '”')
			inside = true
		case '«':
			stack = append(stack, '»')
			inside = true
		case '”', '»':
			if !inside || stack[len(stack)-1] != r {
				return nil, false
			}
			stack = stack[:len(stack)-1]
			inside = true
		}
		mask[i] = inside
	}
	return mask, len(stack) == 0
}

func sentences(text string, mask []bool) []cursor {
	var out []cursor
	start := 0
	add := func(end int) {
		for start < end && text[start] == ' ' {
			start++
		}
		if start < end {
			part := text[start:end]
			out = append(out, cursor{text: part, tokens: tokenize(part, mask[start:end])})
		}
		start = end
	}
	for i := 0; i < len(text); i++ {
		if text[i] != '.' || (i+1 < len(text) && text[i+1] != ' ') {
			continue
		}
		// A leading numeric list marker belongs to its operative sentence.
		if digits(strings.TrimSpace(text[start:i])) && !mask[i] {
			continue
		}
		add(i + 1)
	}
	add(len(text))
	return out
}
