package build

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

// ParseDate parses a source date as a Vietnam civil date.
func ParseDate(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	// VBPL writes a civil date as an offset-free midnight timestamp.
	if date, ok := strings.CutSuffix(value, "T00:00:00"); ok {
		value = date
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err != nil || parsed.Year() == 0 {
			continue
		}
		if layout == time.RFC3339 {
			location, err := time.LoadLocation("Asia/Ho_Chi_Minh")
			if err != nil {
				return nil, fmt.Errorf("load Vietnam time zone: %w", err)
			}
			parsed = parsed.In(location)
		}
		year, month, day := parsed.Date()
		date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
		if year == 0 {
			break
		}
		return &date, nil
	}
	return nil, fmt.Errorf("invalid date")
}

// Status maps a source status code to its frozen class and current flag.
func Status(code string, codes map[string]source.StatusCode) (class string, current bool, known bool) {
	item, ok := codes[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return "unknown", false, false
	}
	return item.Class, item.Current, true
}
