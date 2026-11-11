package textparse

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

func article(s string) (string, bool) {
	r := regexp.MustCompile(`^Điều\s+([0-9]+[a-zA-Z]?)\.(?:\s*(.*))?$`).FindStringSubmatch(s)
	if r == nil {
		return "", false
	}
	title := strings.TrimSpace(r[2])
	if title != "" {
		first := []rune(title)[0]
		if !unicode.IsUpper(first) {
			return "", false
		}
	}
	return strings.ToLower(r[1]), true
}
func structure(s string) (string, string, bool) {
	patterns := []struct{ kind, pattern string }{
		{"phan", `^Phần\s+([IVXLCDMivxlcdm]+|thứ\s+(?:nhất|hai|ba|tư|bốn|năm|sáu|bảy|tám|chín|mười)(?:\s+(?:mươi|mốt|một|hai|ba|tư|bốn|lăm|sáu|bảy|tám|chín|lẻ|linh|trăm|nghìn))*)(?:$|[.:–—-])`},
		{"chuong", `^Chương\s+([IVXLCDMivxlcdm]+|[0-9]+)(?:$|[.:–—-])`},
		{"muc", `^Mục\s+([0-9]+)(?:$|[.:–—-])`},
	}
	for _, p := range patterns {
		if m := regexp.MustCompile(p.pattern).FindStringSubmatch(s); m != nil {
			return p.kind, m[1], true
		}
	}
	return "", "", false
}
func headingCandidate(s string) bool {
	match := regexp.MustCompile(`^Điều\s+[0-9]+[a-zA-Z]?`).FindString(s)
	if match == "" {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(s, match))
	if rest == "" {
		return true
	}
	first := []rune(rest)[0]
	return !unicode.IsLower(first) && first != ',' && first != ';' && first != ')'
}
func capitals(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
}
func instrument(s string) bool {
	if !capitals(s) {
		return false
	}
	for _, prefix := range []string{"QUY CHẾ", "QUY ĐỊNH", "ĐIỀU LỆ"} {
		if s == prefix || strings.HasPrefix(s, prefix+" ") {
			return true
		}
	}
	return false
}
func bareStructure(s string) bool {
	_, _, ok := structure(s)
	return ok && regexp.MustCompile(`^(?:Phần|Chương|Mục)\s+(?:[IVXLCDMivxlcdm]+|[0-9]+|thứ\s+[\pL\s]+)\.?$`).MatchString(s)
}
func signature(s string) bool {
	u := strings.ToUpper(strings.TrimSpace(s))
	return strings.HasPrefix(u, "NƠI NHẬN:") || strings.HasPrefix(u, "TM.") || strings.HasPrefix(u, "KT.") || u == "XÁC THỰC VĂN BẢN HỢP NHẤT" || u == "BỘ TRƯỞNG" || u == "THỦ TƯỚNG" || u == "CHỦ TỊCH" || u == "THỐNG ĐỐC"
}
func appendix(s string) bool {
	s = strings.TrimSpace(s)
	u := strings.ToUpper(s)
	if capitals(s) && (strings.HasPrefix(u, "PHỤ LỤC") || strings.HasPrefix(u, "MẪU SỐ")) {
		return true
	}
	return regexp.MustCompile(`^(?:Phụ lục(?: số)?|Mẫu số)\s+(?:[0-9]+|[IVXLCDMivxlcdm]+)[a-zA-Z]*[.:]?$`).MatchString(s)
}
func instrumentAt(p []Paragraph, i int) bool {
	return instrument(p[i].Text) && (i == 0 || !bareStructure(p[i-1].Text))
}

func outline(s string) bool {
	return regexp.MustCompile(`^(?:[IVXLCDM]+|[0-9]+)\.\s+`).MatchString(s)
}
func sequence(numbers []string) string {
	prev, suffix := 0, byte(0)
	for _, n := range numbers {
		i := 0
		for i < len(n) && n[i] >= '0' && n[i] <= '9' {
			i++
		}
		base, err := strconv.Atoi(n[:i])
		if err != nil {
			return "invalid article number " + n
		}
		if i == len(n) {
			if base != prev+1 {
				return "expected next article after " + strconv.Itoa(prev) + ", found " + n
			}
			prev, suffix = base, 0
		} else {
			c := n[i]
			if base != prev || c < 'a' || c > 'z' || (suffix != 0 && c <= suffix) {
				return "invalid inserted article " + n
			}
			suffix = c
		}
	}
	return ""
}
