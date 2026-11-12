package ocrcheck

func selectEntries(docs []Document, entries []int64) []Document {
	if len(entries) == 0 {
		return docs
	}
	selected := make(map[int64]bool, len(entries))
	for _, id := range entries {
		selected[id] = true
	}
	filtered := make([]Document, 0, len(entries))
	for _, doc := range docs {
		if selected[doc.Entry.ID] {
			filtered = append(filtered, doc)
		}
	}
	return filtered
}
