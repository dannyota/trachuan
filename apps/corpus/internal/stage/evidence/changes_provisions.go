package evidence

import (
	"slices"
	"strings"
)

type provisionChange struct {
	Type       string               `json:"type"`
	Key        string               `json:"key"`
	Anchor     string               `json:"anchor"`
	Change     string               `json:"change"`
	TextSHA256 valueChange[*string] `json:"text_sha256"`
	Provenance bool                 `json:"provenance,omitempty"`
	Ambiguous  bool                 `json:"ambiguous,omitempty"`
}

type classificationChange struct {
	Type       string   `json:"type"`
	Key        string   `json:"key"`
	Anchor     string   `json:"anchor"`
	Added      []string `json:"added"`
	Removed    []string `json:"removed"`
	Provenance bool     `json:"provenance,omitempty"`
	Ambiguous  bool     `json:"ambiguous,omitempty"`
}

type validityChange struct {
	Type       string        `json:"type"`
	Key        string        `json:"key"`
	Anchor     string        `json:"anchor"`
	Added      []validityRow `json:"added"`
	Removed    []validityRow `json:"removed"`
	Provenance bool          `json:"provenance,omitempty"`
	Ambiguous  bool          `json:"ambiguous,omitempty"`
}

type groupReader struct {
	r                 *lineReader
	held              *summaryRecord
	lastKey, lastSeen string
	started           bool
}

func (g *groupReader) read() (*summaryRecord, error) {
	if g.held != nil {
		record := g.held
		g.held = nil
		return record, nil
	}
	record, err := g.r.section(recordProvision)
	if err != nil || record == nil {
		return nil, err
	}
	if g.started && (record.Key < g.lastKey || (record.Key == g.lastKey && record.Anchor <= g.lastSeen)) {
		return nil, refuse("%s provision %s %s is out of order", g.r.name, record.Key, record.Anchor)
	}
	g.started, g.lastKey, g.lastSeen = true, record.Key, record.Anchor
	return record, nil
}

func (g *groupReader) take(key string) ([]summaryRecord, error) {
	var rows []summaryRecord
	for {
		record, err := g.read()
		if err != nil || record == nil {
			return rows, err
		}
		switch strings.Compare(record.Key, key) {
		case -1:
			continue
		case 0:
			rows = append(rows, *record)
		default:
			g.held = record
			return rows, nil
		}
	}
}

type provisionPlan struct {
	in        changeInputs
	keys      []string
	oldFor    map[string]string
	flags     map[string]bool
	preloaded map[string][]summaryRecord
}

func newProvisionPlan(in changeInputs, before, after map[string]summaryRecord, h handOvers) (*provisionPlan, error) {
	p := &provisionPlan{in: in, oldFor: h.oldFor, flags: map[string]bool{}, preloaded: map[string][]summaryRecord{}}
	for _, key := range sortedKeys(after) {
		if !present(after, key) {
			continue
		}
		if old, ok := h.oldFor[key]; ok {
			p.keys = append(p.keys, key)
			p.flags[key] = true
			p.preloaded[old] = nil
		} else if present(before, key) {
			p.keys = append(p.keys, key)
			p.flags[key] = provenanceChanged(before[key], after[key])
		}
	}
	if len(p.preloaded) == 0 {
		return p, nil
	}
	r, err := openSummary(in.predecessor, &identityHeader{})
	if err != nil {
		return nil, err
	}
	defer r.close()
	g := &groupReader{r: r}
	for {
		record, err := g.read()
		if err != nil {
			return nil, err
		}
		if record == nil {
			return p, nil
		}
		if _, ok := p.preloaded[record.Key]; ok {
			p.preloaded[record.Key] = append(p.preloaded[record.Key], *record)
		}
	}
}

func (p *provisionPlan) pass(kind string, out *lineWriter) error {
	current, err := openSummary(p.in.current, &identityHeader{})
	if err != nil {
		return err
	}
	defer current.close()
	previous, err := openSummary(p.in.predecessor, &identityHeader{})
	if err != nil {
		return err
	}
	defer previous.close()
	cur, prev := &groupReader{r: current}, &groupReader{r: previous}
	for _, key := range p.keys {
		after, err := cur.take(key)
		if err != nil {
			return err
		}
		before := p.preloaded[p.oldFor[key]]
		if _, paired := p.oldFor[key]; !paired {
			if before, err = prev.take(key); err != nil {
				return err
			}
		}
		if err = compareProvisions(out, kind, key, before, after, p.flags[key]); err != nil {
			return err
		}
	}
	return nil
}

func compareProvisions(out *lineWriter, kind, key string, before, after []summaryRecord, provenance bool) error {
	i, j := 0, 0
	for i < len(before) || j < len(after) {
		var record any
		switch {
		case j == len(after) || (i < len(before) && before[i].Anchor < after[j].Anchor):
			b := before[i]
			if kind == changeProvision {
				record = provisionChange{Type: changeProvision, Key: key, Anchor: b.Anchor, Change: "removed", TextSHA256: valueChange[*string]{Before: &b.TextSHA256}, Provenance: provenance, Ambiguous: b.Ambiguous}
			}
			i++
		case i == len(before) || after[j].Anchor < before[i].Anchor:
			a := after[j]
			if kind == changeProvision {
				record = provisionChange{Type: changeProvision, Key: key, Anchor: a.Anchor, Change: "added", TextSHA256: valueChange[*string]{After: &a.TextSHA256}, Provenance: provenance, Ambiguous: a.Ambiguous}
			}
			j++
		default:
			record = pairedChange(kind, key, before[i], after[j], provenance)
			i++
			j++
		}
		if record != nil {
			if err := out.write(record); err != nil {
				return err
			}
		}
	}
	return nil
}

func pairedChange(kind, key string, b, a summaryRecord, provenance bool) any {
	ambiguous := b.Ambiguous || a.Ambiguous
	switch kind {
	case changeProvision:
		if b.TextSHA256 != a.TextSHA256 {
			return provisionChange{Type: changeProvision, Key: key, Anchor: a.Anchor, Change: "changed", TextSHA256: valueChange[*string]{Before: &b.TextSHA256, After: &a.TextSHA256}, Provenance: provenance, Ambiguous: ambiguous}
		}
	case changeClassification:
		added, removed := setDiff(b.Nodes, a.Nodes), setDiff(a.Nodes, b.Nodes)
		if len(added) > 0 || len(removed) > 0 {
			return classificationChange{Type: changeClassification, Key: key, Anchor: a.Anchor, Added: added, Removed: removed, Provenance: provenance, Ambiguous: ambiguous}
		}
	case changeValidity:
		added, removed := validityDiff(b.Validity, a.Validity)
		if len(added) > 0 || len(removed) > 0 {
			return validityChange{Type: changeValidity, Key: key, Anchor: a.Anchor, Added: added, Removed: removed, Provenance: provenance, Ambiguous: ambiguous}
		}
	}
	return nil
}

func setDiff(before, after []string) []string {
	out := []string{}
	for _, code := range after {
		if !slices.Contains(before, code) {
			out = append(out, code)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func validityKey(v validityRow) string {
	optional := func(s *string) string {
		if s == nil {
			return "\x01"
		}
		return *s
	}
	return v.StatusClass + "\x00" + optional(v.EffFrom) + "\x00" + optional(v.EffTo) + "\x00" + v.Method
}

func validityDiff(before, after []validityRow) (added, removed []validityRow) {
	counts := map[string]int{}
	for _, v := range before {
		counts[validityKey(v)]++
	}
	added, removed = []validityRow{}, []validityRow{}
	for _, v := range after {
		if counts[validityKey(v)] > 0 {
			counts[validityKey(v)]--
			continue
		}
		added = append(added, v)
	}
	for _, v := range before {
		if counts[validityKey(v)] > 0 {
			counts[validityKey(v)]--
			removed = append(removed, v)
		}
	}
	slices.SortFunc(added, compareValidity)
	slices.SortFunc(removed, compareValidity)
	return added, removed
}
