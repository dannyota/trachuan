package retrieval

func DocumentValidity(doc Document, date Date) (Validity, []Gap) {
	return stamp(doc, Arms{Date: date})
}
