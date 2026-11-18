package evidence

import "slices"

type relationChange struct {
	Type   string      `json:"type"`
	Change string      `json:"change"`
	Kind   string      `json:"kind"`
	Method string      `json:"method"`
	Source relationEnd `json:"source"`
	Target relationEnd `json:"target"`
	Count  int         `json:"count"`
}

type relationCount struct {
	identity relationIdentity
	count    int
}

func remapEnd(end *relationEnd, remap map[string]string) {
	if end.Key == nil {
		return
	}
	if replacement, ok := remap[*end.Key]; ok {
		end.Key = &replacement
	}
}

func readRelations(path string, remap map[string]string) (map[string]*relationCount, error) {
	r, err := openSummary(path, &identityHeader{})
	if err != nil {
		return nil, err
	}
	defer r.close()
	out := map[string]*relationCount{}
	for {
		record, err := r.section(recordRelation)
		if err != nil {
			return nil, err
		}
		if record == nil {
			break
		}
		if record.Count < 1 {
			return nil, refuse("%s relation has count %d", path, record.Count)
		}
		identity := record.relation()
		remapEnd(&identity.Source, remap)
		remapEnd(&identity.Target, remap)
		data, err := encodeLine(identity)
		if err != nil {
			return nil, err
		}
		entry := out[string(data)]
		if entry == nil {
			entry = &relationCount{identity: identity}
			out[string(data)] = entry
		}
		entry.count += record.Count
	}
	return out, nil
}

func writeRelationChanges(in changeInputs, h handOvers, out *lineWriter) error {
	after, err := readRelations(in.current, nil)
	if err != nil {
		return err
	}
	before, err := readRelations(in.predecessor, h.newFor)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(after)+len(before))
	for key := range after {
		keys = append(keys, key)
	}
	for key := range before {
		if after[key] == nil {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		var entry *relationCount
		diff := 0
		if a := after[key]; a != nil {
			entry, diff = a, a.count
		}
		if b := before[key]; b != nil {
			entry, diff = b, diff-b.count
		}
		change := "added"
		if diff < 0 {
			change, diff = "removed", -diff
		}
		if diff == 0 {
			continue
		}
		id := entry.identity
		if err = out.write(relationChange{Type: changeRelation, Change: change, Kind: id.Kind, Method: id.Method, Source: id.Source, Target: id.Target, Count: diff}); err != nil {
			return err
		}
	}
	return nil
}
