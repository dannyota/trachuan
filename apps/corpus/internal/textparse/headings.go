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
	r := regexp.MustCompile(`^(Phần|Chương|Mục)\s+([\pL0-9]+)(?:[.\s]|$)`).FindStringSubmatch(s)
	if r == nil {
		return "", "", false
	}
	kind := map[string]string{"Phần": "phan", "Chương": "chuong", "Mục": "muc"}[r[1]]
	return kind, r[2], true
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
	return regexp.MustCompile(`^(Phần|Chương|Mục)\s+[\pL0-9]+\.?$`).MatchString(s)
}
func signature(s string) bool {
	u := strings.ToUpper(strings.TrimSpace(s))
	return strings.HasPrefix(u, "NƠI NHẬN:") || strings.HasPrefix(u, "TM.") || strings.HasPrefix(u, "KT.") || u == "XÁC THỰC VĂN BẢN HỢP NHẤT" || u == "BỘ TRƯỞNG" || u == "THỦ TƯỚNG" || u == "CHỦ TỊCH" || u == "THỐNG ĐỐC"
}
func appendix(s string) bool {
	u := strings.ToUpper(strings.TrimSpace(s))
	return strings.HasPrefix(u, "PHỤ LỤC") || strings.HasPrefix(u, "MẪU SỐ")
}
func instrumentAt(p []Paragraph, i int) bool {
	return instrument(p[i].Text) && (i == 0 || !bareStructure(p[i-1].Text))
}

func outline(s string) bool { return regexp.MustCompile(`^(?:[IVXLCDM]+|[0-9]+)\.\s+`).MatchString(s) }
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
