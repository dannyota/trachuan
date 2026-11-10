package build

// Presence returns the keys of documents that belong to this run's corpus.
func (i Inventory) Presence() map[string]bool {
	eligible := map[string]bool{}
	if i.Version == 2 {
		for _, entry := range i.Membership.Entries {
			if entry.Disposition == "active" {
				eligible[entry.DocumentKey] = true
			}
		}
	} else {
		for _, key := range i.ExpectedKeys {
			eligible[key] = true
		}
		for _, gap := range i.Gaps {
			if gap.Kind == "withdrawn" || gap.Kind == "scope_removed" {
				delete(eligible, gap.ItemKey)
			}
		}
	}
	present := map[string]bool{}
	for _, item := range i.Items {
		if item.Stage == "fetch" && item.State == "done" && eligible[item.ItemKey] {
			present[item.ItemKey] = true
		}
	}
	return present
}
