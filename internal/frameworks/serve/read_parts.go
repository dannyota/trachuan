package serve

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

func serializedSize(value any) int {
	raw, _ := json.Marshal(value)
	return utf8.RuneCount(raw)
}

func cutBoundary(text []rune, limit int) int {
	if limit >= len(text) {
		return len(text)
	}
	if limit <= 0 {
		return 0
	}
	if unicode.IsSpace(text[limit]) {
		return limit
	}
	for i := limit; i > 0; i-- {
		if unicode.IsSpace(text[i-1]) {
			return i
		}
	}
	end := limit
	for end > 0 && unicode.Is(unicode.Mn, text[end]) {
		end--
	}
	if end == 0 {
		return limit
	}
	return end
}

func textUnits(title, body string, notes, tables []string) []ReadUnit {
	units := []ReadUnit{{Kind: "title_original", Text: title}}
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if line != "" {
			units = append(units, ReadUnit{Kind: "body", Text: line})
		}
	}
	for _, note := range notes {
		units = append(units, ReadUnit{Kind: "note", Text: note})
	}
	for _, table := range tables {
		units = append(units, ReadUnit{Kind: "table", Text: table})
	}
	return units
}

func unitText(unit ReadUnit) string {
	switch unit.Kind {
	case "title_original", "body", "note", "table":
		return unit.Text
	case "mapping":
		if unit.Edge != nil {
			return unit.Edge.Evidence
		}
	}
	return ""
}

func withText(unit ReadUnit, text string) ReadUnit {
	if unit.Kind == "mapping" && unit.Edge != nil {
		edge := *unit.Edge
		edge.Evidence = text
		unit.Edge = &edge
	} else {
		unit.Text = text
	}
	return unit
}

func readUnitParts(unit ReadUnit, capRunes int) ([]ReadUnit, bool) {
	if serializedSize(unit) <= capRunes {
		return []ReadUnit{unit}, true
	}
	text := unitText(unit)
	if text == "" {
		return nil, false
	}
	for total := 1; ; {
		var parts []ReadUnit
		for remaining := []rune(text); len(remaining) > 0; {
			base := withText(unit, "")
			base.Part, base.Parts = len(parts)+1, total
			lo, hi := 0, min(len(remaining), capRunes)
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if serializedSize(withText(base, string(remaining[:mid]))) <= capRunes {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
			if lo == 0 {
				return nil, false
			}
			end := cutBoundary(remaining, lo)
			parts = append(parts, withText(base, string(remaining[:end])))
			remaining = remaining[end:]
		}
		if len(parts) == total {
			return parts, true
		}
		total = len(parts)
	}
}

func (e *Engine) readPage(units []ReadUnit, c continuation, p projection, budget, partSize int) (ReadItemResult, int) {
	result := ReadItemResult{Handle: c.Handle}
	if c.Unit < 0 || c.Unit >= len(units) || c.Part < 0 {
		result.Error = "invalid_next"
		return result, 0
	}
	used := 0
	for i := c.Unit; i < len(units); i++ {
		parts, ok := readUnitParts(units[i], partSize)
		if !ok {
			result.Error = "unit_too_large"
			return result, used
		}
		start := 0
		if i == c.Unit {
			start = c.Part
		}
		if start >= len(parts) {
			result.Error = "invalid_next"
			return result, used
		}
		for j := start; j < len(parts); j++ {
			cost := serializedSize(parts[j])
			if used+cost > budget {
				c.Unit, c.Part, c.Access = i, j, p.version
				result.Next = e.encodeContinuation(c)
				if used == 0 {
					result.NotRead = "call_cap"
				}
				return result, used
			}
			result.Units = append(result.Units, parts[j])
			used += cost
		}
	}
	result.Complete = true
	return result, used
}
