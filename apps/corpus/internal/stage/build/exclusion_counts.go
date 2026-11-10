package build

func exclusionCounts(inv Inventory) map[string]int64 {
	counts := map[string]int64{"excluded_withdrawn": 0, "excluded_scope_removed": 0, "excluded_fetch_gap": 0}
	reasons := make(map[string]string)
	for _, gap := range inv.Gaps {
		if gap.Kind == "withdrawn" || (gap.Kind == "scope_removed" && reasons[gap.ItemKey] == "") {
			reasons[gap.ItemKey] = gap.Kind
		}
	}
	states := make(map[string]string)
	for _, item := range inv.Items {
		if item.Stage == "fetch" {
			states[item.ItemKey] = item.State
		}
	}
	for _, key := range inv.ExpectedKeys {
		switch reasons[key] {
		case "withdrawn":
			counts["excluded_withdrawn"]++
		case "scope_removed":
			counts["excluded_scope_removed"]++
		default:
			if states[key] != "done" {
				counts["excluded_fetch_gap"]++
			}
		}
	}
	return counts
}
