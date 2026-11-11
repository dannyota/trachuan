package textparse

// quotedParagraphs marks paragraph starts inside quotes and unbalanced marks.
func quotedParagraphs(p []Paragraph) ([]bool, []int) {
	quoted := make([]bool, len(p))
	failed := make([]bool, len(p))
	depth, directionalStart, straightStart := 0, -1, -1
	straight := false
	for i, para := range p {
		quoted[i] = depth > 0 || straight
		for _, mark := range para.Text {
			switch mark {
			case '“', '«':
				if depth == 0 {
					directionalStart = i
				}
				depth++
			case '”', '»':
				if depth == 0 {
					failed[i] = true
				} else {
					depth--
					if depth == 0 {
						directionalStart = -1
					}
				}
			case '"':
				straight = !straight
				if straight {
					straightStart = i
				} else {
					straightStart = -1
				}
			}
		}
	}
	if depth > 0 {
		failed[directionalStart] = true
	}
	if straight {
		failed[straightStart] = true
	}
	var failures []int
	for i, bad := range failed {
		if bad {
			failures = append(failures, i)
		}
	}
	return quoted, failures
}
