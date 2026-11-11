package textparse

// quotedParagraphs marks paragraph starts inside quotes and unbalanced marks.
func quotedParagraphs(p []Paragraph) ([]bool, []int) {
	quoted := make([]bool, len(p))
	var failures []int
	depth, start := 0, -1
	for i, para := range p {
		quoted[i] = depth > 0
		for _, mark := range para.Text {
			switch mark {
			case '“', '«':
				if depth == 0 {
					start = i
				}
				depth++
			case '”', '»':
				if depth == 0 {
					failures = append(failures, i)
				} else {
					depth--
					if depth == 0 {
						start = -1
					}
				}
			case '"':
				switch depth {
				case 0:
					depth, start = 1, i
				case 1:
					depth, start = 0, -1
				}
			}
		}
	}
	if depth > 0 {
		failures = append(failures, start)
	}
	return quoted, failures
}
