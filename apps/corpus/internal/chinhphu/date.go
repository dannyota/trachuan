package chinhphu

import (
	"strings"
	"time"
)

func sourceDate(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) != 10 || value[2] != '-' || value[5] != '-' {
		return value
	}
	for i := range len(value) {
		if i != 2 && i != 5 && (value[i] < '0' || value[i] > '9') {
			return value
		}
	}
	date, err := time.Parse("02-01-2006", value)
	if err != nil || date.Year() == 0 {
		return value
	}
	return date.Format("2006-01-02")
}
