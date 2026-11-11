package retrieval

import "strings"

type readMatch struct {
	count int
	first int
}

// findReadChild searches forward, then checks earlier and repeated occurrences.
// Count follows strings.Count's non-overlapping rule, capped at two.
func findReadChild(raw, text string, after int) readMatch {
	result := readMatch{first: -1}
	position := strings.Index(raw[after:], text)
	if position < 0 {
		result.first = strings.Index(raw, text)
	} else {
		position += after
		result.first = position
		if after > 0 {
			// Include overlapping starts before position without including position.
			if earlier := strings.Index(raw[:position+len(text)-1], text); earlier >= 0 {
				result.first = earlier
			}
		}
	}
	if result.first < 0 {
		return result
	}
	result.count = 1
	if strings.Contains(raw[result.first+len(text):], text) {
		result.count = 2
	}
	return result
}
