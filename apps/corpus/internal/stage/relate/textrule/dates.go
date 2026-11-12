package textrule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func civilDate(day, month, year string) (string, bool) {
	if !digits(day) || !digits(month) || !digits(year) || len(day) > 2 || len(month) > 2 || len(year) != 4 {
		return "", false
	}
	d, _ := strconv.Atoi(day)
	m, _ := strconv.Atoi(month)
	y, _ := strconv.Atoi(year)
	if y == 0 || m < 1 || m > 12 || d < 1 {
		return "", false
	}
	date := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if date.Year() != y || int(date.Month()) != m || date.Day() != d {
		return "", false
	}
	return fmt.Sprintf("%04d-%02d-%02d", y, m, d), true
}

// date accepts Vietnamese civil dates; ranges also accept shared source formats.
func (c *cursor) date(tail bool) (string, bool) {
	start := c.pos
	if start+4 < len(c.tokens) {
		parts := c.tokens[start : start+5]
		allOutside := true
		for _, part := range parts {
			allOutside = allOutside && !part.quoted
		}
		if allOutside && ((parts[1].word == "/" && parts[3].word == "/") || (parts[1].word == "tháng" && parts[3].word == "năm")) {
			date, ok := civilDate(parts[0].word, parts[2].word, parts[4].word)
			if ok {
				c.pos += 5
			}
			return date, ok
		}
	}
	if tail || start >= len(c.tokens) || c.tokens[start].quoted {
		return "", false
	}
	// ISO dates and timestamps have no spaces. Keep the complete source value.
	end := start + 1
	for end < len(c.tokens) && !c.tokens[end].quoted && c.tokens[end-1].end == c.tokens[end].start {
		end++
	}
	raw := strings.TrimSuffix(c.raw(start, end), ".")
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04:05", time.RFC3339} {
		if layout == "2006-01-02T15:04:05" && len(raw) != len(layout) {
			continue
		}
		parsed, err := time.Parse(layout, raw)
		if err != nil || parsed.Year() == 0 {
			continue
		}
		if layout == time.RFC3339 {
			location, err := time.LoadLocation("Asia/Ho_Chi_Minh")
			if err != nil {
				return "", false
			}
			parsed = parsed.In(location)
		}
		if parsed.Year() == 0 {
			return "", false
		}
		c.pos = end
		return parsed.Format("2006-01-02"), true
	}
	return "", false
}

func (c *cursor) dateRange() bool {
	if !c.take("từ", "ngày") && !c.take("kể", "từ", "ngày") {
		return true
	}
	if _, ok := c.date(false); !ok {
		return false
	}
	if c.take("đến") {
		c.take("hết")
		if !c.take("ngày") {
			return false
		}
		if _, ok := c.date(false); !ok {
			return false
		}
	}
	return true
}

func (c *cursor) tail() (*string, string) {
	// Inspect the whole tail first so extra targets cannot hide behind bad dates.
	for i := c.pos; i < len(c.tokens); i++ {
		word := c.tokens[i].word
		if word == "điều" || word == "khoản" || word == "điểm" {
			return nil, "relation_text_multi_target"
		}
		probe := *c
		probe.pos = i
		if _, ok := probe.number(); ok {
			return nil, "relation_text_multi_target"
		}
	}
	var stated *string
	for c.pos < len(c.tokens) {
		if !c.take("ngày") {
			c.pos++
			continue
		}
		if c.pos >= len(c.tokens) || !digits(c.tokens[c.pos].word) {
			continue
		}
		date, ok := c.date(true)
		if !ok || stated != nil {
			return nil, "relation_text_grammar"
		}
		stated = &date
	}
	return stated, ""
}
