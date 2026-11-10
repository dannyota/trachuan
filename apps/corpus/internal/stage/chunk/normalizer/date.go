package normalizer

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
)

// Version identifies the shared date normalizer.
const Version = "corpus-normalizer/3"

// ParseDate parses a source date as a Vietnam civil date.
func ParseDate(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "2006-01-02T15:04:05", time.RFC3339} {
		if layout == "2006-01-02T15:04:05" && len(value) != len(layout) {
			continue
		}
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
