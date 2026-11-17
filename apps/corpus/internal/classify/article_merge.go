package classify

import "sort"

func mergeArticleFamilies(records []ArticleRecord) []ArticleRecord {
	type merged struct {
		record    ArticleRecord
		succeeded bool
	}
	byKey := map[string]*merged{}
	var result []ArticleRecord
	for _, record := range records {
		key := record.DocumentKey + "/" + record.Anchor
		m := byKey[key]
		if m == nil {
			m = &merged{record: ArticleRecord{DocumentKey: record.DocumentKey, Anchor: record.Anchor, Outcome: emptyArticle(record.Outcome.Outcome)}}
			m.record.Outcome.FailedFamilies = new(int)
			byKey[key] = m
		}
		out := record.Outcome
		if out.Outcome == "ok" {
			m.succeeded = true
			m.record.Outcome.Codes = append(m.record.Outcome.Codes, out.Codes...)
		}
		if out.FailedFamilies != nil {
			*m.record.Outcome.FailedFamilies += *out.FailedFamilies
		}
		m.record.Outcome.Dropped = append(m.record.Outcome.Dropped, out.Dropped...)
	}
	for _, m := range byKey {
		if m.succeeded {
			m.record.Outcome.Outcome = "ok"
		} else if *m.record.Outcome.FailedFamilies > 0 {
			m.record.Outcome.Outcome = "invalid_response"
		}
		sort.Slice(m.record.Outcome.Codes, func(i, j int) bool { return m.record.Outcome.Codes[i].Code < m.record.Outcome.Codes[j].Code })
		result = append(result, m.record)
	}
	return result
}
