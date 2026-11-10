package build

import (
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk/normalizer"
)

// ParseDate parses a source date as a Vietnam civil date.
func ParseDate(raw string) (*time.Time, error) { return normalizer.ParseDate(raw) }

// Status maps a source status code to its frozen class and current flag.
func Status(code string, codes map[string]source.StatusCode) (class string, current bool, known bool) {
	item, ok := codes[strings.ToUpper(strings.TrimSpace(code))]
	if !ok {
		return "unknown", false, false
	}
	return item.Class, item.Current, true
}
