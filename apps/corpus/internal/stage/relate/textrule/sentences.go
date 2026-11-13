package textrule

import "unicode/utf8"

// quoteMask marks quoted bytes, including delimiters, across the whole provision.
func quoteMask(text string) ([]bool, bool) {
	mask := make([]bool, len(text))
	var stack []rune
	valid := true
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
				valid = false
			} else {
				stack = stack[:len(stack)-1]
			}
			inside = true
		}
		for j := i; j < i+utf8.RuneLen(r); j++ {
			mask[j] = inside
		}
	}
	return mask, valid && len(stack) == 0
}

func quotationEnd(text string, start int) int {
	var stack []rune
	for offset, r := range text[start:] {
		switch r {
		case '"', '\'':
			if len(stack) > 0 && stack[len(stack)-1] == r {
				stack = stack[:len(stack)-1]
			} else {
				stack = append(stack, r)
			}
		case '“':
			stack = append(stack, '”')
		case '«':
			stack = append(stack, '»')
		case '”', '»':
			if len(stack) == 0 || stack[len(stack)-1] != r {
				return -1
			}
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			return start + offset + utf8.RuneLen(r)
		}
	}
	return -1
}
