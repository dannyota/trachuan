package ocrcheck

// DocumentSummary contains safe pilot metrics without document content.
// EntryID identifies the immutable OCR row across direct and batch invocations.
type DocumentSummary struct {
	EntryID                                 int64
	Pages                                   int
	InputTokens, OutputTokens, CostMicroUSD int64
	Outcome                                 string
}

func (s *Summary) record(doc Document, pages int, u Usage, outcome string) {
	if u.APIErrorType != "" && (outcome == "api_error" || outcome == "uncertain:api_error") {
		outcome += ":" + u.APIErrorType
	}
	s.Documents = append(s.Documents, DocumentSummary{EntryID: doc.Entry.ID, Pages: pages, InputTokens: u.Input, OutputTokens: u.Output, CostMicroUSD: u.charge(), Outcome: outcome})
}

func (s *Summary) recordPending(a Attempt) {
	s.Documents = append(s.Documents, DocumentSummary{EntryID: a.EntryID, Pages: a.Usage.PDFPages, InputTokens: a.Usage.CountedInput, OutputTokens: a.Usage.Output, CostMicroUSD: a.Usage.charge(), Outcome: a.State})
}
