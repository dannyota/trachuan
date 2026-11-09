package extract

import (
	"regexp"
	"strconv"
	"strings"
)

func nodeKind(level string, ptype int) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "part":
		return "phan"
	case "chapter":
		return "chuong"
	case "section":
		return "muc"
	case "article":
		return "dieu"
	case "clause":
		return "khoan"
	case "point":
		return "diem"
	}
	switch ptype {
	case 1:
		return "phan"
	case 2:
		return "chuong"
	case 3, 4:
		return "muc"
	case 5:
		return "dieu"
	case 6:
		return "khoan"
	case 7:
		return "diem"
	}
	return "other"
}

func titleParts(kind, title string, position int) (string, string, string, int) {
	title = strings.TrimSpace(textFromHTML(title))
	prefixes := map[string]string{"phan": "Phần", "chuong": "Chương", "muc": "Mục", "dieu": "Điều", "khoan": "Khoản", "diem": "Điểm"}
	prefix := prefixes[kind]
	if kind == "other" {
		return title, "", "other-" + strconv.Itoa(position), 0
	}
	pattern := `(?i)^` + prefix + `\s+([^\s.:)]+)(?:[.:)\s]+(.*))?$`
	if kind == "khoan" || kind == "diem" {
		pattern = `(?i)^(?:` + prefix + `\s+)?([^\s.:)]+)(?:[.:)\s]+(.*))?$`
	}
	match := regexp.MustCompile(pattern).FindStringSubmatch(strings.ReplaceAll(title, "\n", " "))
	token := strconv.Itoa(position)
	label, heading, number := prefix+" "+token, "", 0
	if match != nil {
		token = match[1]
		label = prefix + " " + token
		heading = strings.TrimSpace(match[2])
		if kind == "phan" && strings.EqualFold(token, "thứ") {
			fields := strings.Fields(heading)
			if len(fields) > 0 {
				label += " " + fields[0]
				ordinals := map[string]string{"nhất": "1", "hai": "2", "ba": "3", "tư": "4", "năm": "5", "sáu": "6", "bảy": "7", "tám": "8", "chín": "9", "mười": "10"}
				if value, ok := ordinals[strings.ToLower(fields[0])]; ok {
					token = value
				} else {
					token = strconv.Itoa(position)
				}
				heading = strings.TrimSpace(strings.TrimPrefix(heading, fields[0]))
			}
		}
		if kind == "dieu" {
			digits := regexp.MustCompile(`^[0-9]+`).FindString(token)
			number, _ = strconv.Atoi(digits)
		}
	}
	return label, heading, kind + "-" + strings.ToLower(token), number
}
