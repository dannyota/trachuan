package validity

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

const datePattern = `(?:\d{1,2}\s+tháng\s+\d{1,2}\s+năm\s+\d{4}|\d{1,2}/\d{1,2}/\d{4})`

func parseDate(s string) (Date, bool) {
	nums := regexp.MustCompile(`\d+`).FindAllString(s, -1)
	if len(nums) != 3 {
		return Date{}, false
	}
	d, _ := strconv.Atoi(nums[0])
	m, _ := strconv.Atoi(nums[1])
	y, _ := strconv.Atoi(nums[2])
	if y < 1 || m < 1 || m > 12 || d < 1 {
		return Date{}, false
	}
	date := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if date.Year() != y || int(date.Month()) != m || date.Day() != d {
		return Date{}, false
	}
	return Date{y, m, d}, true
}

func parseCommencement(documentType, sentence string) (Date, bool, bool) {
	pattern := `(?i)^\s*` + regexp.QuoteMeta(strings.TrimSpace(documentType)) + `\s+này\s+có\s+hiệu\s+lực(?:\s+thi\s+hành)?(?:\s+kể)?\s+từ\s+ngày\s+(` + datePattern + `)(?:\s*,\s*(trừ)(?:\s+[^.]*)?)?\s*\.\s*$`
	match := regexp.MustCompile(pattern).FindStringSubmatch(sentence)
	if match == nil {
		return Date{}, false, false
	}
	date, ok := parseDate(match[1])
	return date, match[2] != "", ok
}
