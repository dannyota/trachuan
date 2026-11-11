package evaluation

// Coverage identifies an absent expectation and its searched numbers.
type Coverage struct {
	CaseID        string   `json:"case_id"`
	CitationIndex int      `json:"citation_index"`
	Numbers       []string `json:"numbers"`
}
