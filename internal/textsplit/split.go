package textsplit

import "unicode"

type Range struct {
	Start int
	End   int
}

type Event struct {
	Ranges      []Range
	Separator   rune
	AttachLead  bool
	ReleaseLead bool
}

func Rough(n int) int {
	if n == 0 {
		return 0
	}
	if n < 2 {
		return 1
	}
	return n / 2
}

func Split(body []rune, lead int, limit int) []Event {
	whole := trim(body, Range{0, len(body)})
	if whole.Start == whole.End {
		return nil
	}
	if Rough(joinedLength(lead, whole.End-whole.Start)) <= limit {
		return []Event{{Ranges: []Range{whole}, AttachLead: lead > 0}}
	}
	var lines []Range
	start := whole.Start
	for i := whole.Start; i <= whole.End; i++ {
		if i == whole.End || body[i] == '\n' {
			line := trim(body, Range{start, i})
			if line.Start < line.End {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	var events []Event
	var current Event
	currentLength := 0
	flush := func() {
		if currentLength > 0 {
			current.AttachLead = lead > 0
			events = append(events, current)
			current = Event{}
			currentLength = 0
		}
	}
	for _, line := range lines {
		lineLength := line.End - line.Start
		if Rough(joinedLength(lead, lineLength)) <= limit {
			candidateLength := joinedLength(currentLength, lineLength)
			if currentLength > 0 && Rough(joinedLength(lead, candidateLength)) > limit {
				flush()
				candidateLength = lineLength
			}
			current.Ranges = append(current.Ranges, line)
			current.Separator = '\n'
			currentLength = candidateLength
			continue
		}
		flush()
		var words []Range
		for i := line.Start; i < line.End; {
			for i < line.End && unicode.IsSpace(body[i]) {
				i++
			}
			start = i
			for i < line.End && !unicode.IsSpace(body[i]) {
				i++
			}
			if start < i {
				words = append(words, Range{start, i})
			}
		}
		for _, word := range words {
			wordLength := word.End - word.Start
			if lead > 0 && Rough(joinedLength(lead, wordLength)) > limit {
				events = append(events, Event{ReleaseLead: true})
				lead = 0
			}
			candidateLength := joinedLength(currentLength, wordLength)
			if currentLength > 0 && Rough(joinedLength(lead, candidateLength)) > limit {
				flush()
				candidateLength = wordLength
			}
			current.Ranges = append(current.Ranges, word)
			current.Separator = ' '
			currentLength = candidateLength
		}
		flush()
	}
	flush()
	return events
}

func joinedLength(a, b int) int {
	if a > 0 && b > 0 {
		return a + 1 + b
	}
	return a + b
}

func trim(body []rune, r Range) Range {
	for r.Start < r.End && unicode.IsSpace(body[r.Start]) {
		r.Start++
	}
	for r.End > r.Start && unicode.IsSpace(body[r.End-1]) {
		r.End--
	}
	return r
}
