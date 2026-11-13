package textparse

import (
	"regexp"
	"strconv"
	"unicode"
	"unicode/utf8"
)

const vietnameseOrder = "abcdđeghiklmnopqrstuvxy"

type childGrammar struct{ clause, point, clauseCandidate, pointCandidate, footnote *regexp.Regexp }

func newChildGrammar() childGrammar {
	return childGrammar{
		footnote:        regexp.MustCompile(`^\[[0-9]+\]`),
		clause:          regexp.MustCompile(`^([0-9]{1,3}\p{Ll}?)([.-])(.*)$`),
		point:           regexp.MustCompile(`^([abcdđeghiklmnopqrstuvxy][0-9]{0,2})([).\-])(.*)$`),
		clauseCandidate: regexp.MustCompile(`^[0-9]+\p{L}?\s*[.)\-:]`),
		pointCandidate:  regexp.MustCompile(`^(?:\p{Ll}[0-9]*[).\-:]|\p{Lu}[0-9]*\))`),
	}
}

func (g childGrammar) read(text string) (childStart, string) {
	if m := g.clause.FindStringSubmatch(text); m != nil {
		first, _ := utf8.DecodeRuneInString(m[3])
		valid := len(m[3]) > 0 && unicode.IsSpace(first)
		if m[2] == "." {
			valid = valid || m[3] == "" || unicode.IsUpper(first) || g.footnote.MatchString(m[3])
		}
		if valid {
			return childStart{kind: "khoan", token: m[1], style: m[2]}, ""
		}
	}
	if g.clauseCandidate.MatchString(text) {
		return childStart{}, "clause_candidate"
	}
	if m := g.point.FindStringSubmatch(text); m != nil {
		first, _ := utf8.DecodeRuneInString(m[3])
		if m[2] == ")" || (m[3] != "" && unicode.IsSpace(first)) {
			return childStart{kind: "diem", token: m[1], style: m[2]}, ""
		}
	}
	if g.pointCandidate.MatchString(text) {
		return childStart{}, "point_candidate"
	}
	return childStart{}, ""
}

func checkChildren(starts []childStart, point bool) string {
	kind := "clause"
	if point {
		kind = "point"
	}
	previous, suffix := 0, 0
	for _, s := range starts {
		if s.style != starts[0].style {
			return kind + "_style"
		}
		if point {
			letter, width := utf8.DecodeRuneInString(s.token)
			order := []rune(vietnameseOrder)
			base := 0
			for i, r := range order {
				if r == letter {
					base = i + 1
					break
				}
			}
			inserted := 0
			if width < len(s.token) {
				inserted, _ = strconv.Atoi(s.token[width:])
				if inserted == 0 {
					return "point_sequence"
				}
			}
			if inserted == 0 {
				if base != previous+1 {
					return "point_sequence"
				}
				previous, suffix = base, 0
			} else {
				if base != previous || inserted != suffix+1 {
					return "point_sequence"
				}
				suffix = inserted
			}
			continue
		}
		i := 0
		for i < len(s.token) && s.token[i] >= '0' && s.token[i] <= '9' {
			i++
		}
		base, _ := strconv.Atoi(s.token[:i])
		if i == len(s.token) {
			if base != previous+1 {
				return "clause_sequence"
			}
			previous, suffix = base, 0
		} else {
			letter, _ := utf8.DecodeRuneInString(s.token[i:])
			inserted := 0
			for j, r := range []rune(vietnameseOrder) {
				if r == letter {
					inserted = j + 1
					break
				}
			}
			if base != previous || inserted == 0 || inserted != suffix+1 {
				return "clause_sequence"
			}
			suffix = inserted
		}
	}
	return ""
}
