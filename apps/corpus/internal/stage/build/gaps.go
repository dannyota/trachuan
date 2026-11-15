package build

import (
	"context"
	"fmt"
	"sort"
)

func (l *loader) loadGaps(ctx context.Context) error {
	sort.Slice(l.gaps, func(i, j int) bool {
		a, b := l.gaps[i], l.gaps[j]
		if a.Stage != b.Stage {
			return a.Stage < b.Stage
		}
		if a.ItemKey != b.ItemKey {
			return a.ItemKey < b.ItemKey
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Detail < b.Detail
	})
	var previous *InventoryGap
	for _, g := range l.gaps {
		if previous != nil && previous.Stage == g.Stage && previous.ItemKey == g.ItemKey && previous.Kind == g.Kind && previous.Detail == g.Detail {
			continue
		}
		var key any
		if g.ItemKey != "" {
			key = g.ItemKey
		}
		message := ""
		if g.Kind == "baseline_served" {
			if g.Detail == "unknown" {
				message = "The source fetch date for this text is unknown."
			} else {
				message = "This text is from the source as fetched on " + g.Detail + "."
			}
		}
		_, err := l.tx.Exec(ctx, `INSERT INTO coverage_gap(id,kind,document_id,document_key,node_id,stage,item_key,detail,message,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'')`, l.counts.Gaps+1, g.Kind, docID(l.docs[g.ItemKey]), key, g.NodeID, g.Stage, g.ItemKey, g.Detail, message)
		if err != nil {
			return fmt.Errorf("insert coverage gap: %w", err)
		}
		l.counts.Gaps++
		copy := g
		previous = &copy
	}
	return nil
}

func (l *loader) addMembershipGaps() {
	dates := map[string]string{}
	for _, d := range l.inv.Provenance.DetailTimes {
		if d.FetchedAt != nil {
			dates[d.DocumentKey] = d.FetchedAt.UTC().Format("2006-01-02")
		}
	}
	seen := map[string]bool{}
	for _, g := range l.gaps {
		if g.Stage != "fetch" || g.Kind != "source_unavailable" || l.items["fetch"][g.ItemKey].State != "done" || seen[g.ItemKey] {
			continue
		}
		seen[g.ItemKey] = true
		date := dates[g.ItemKey]
		if date == "" {
			date = "unknown"
		}
		l.addGap("build", g.ItemKey, "baseline_served", date)
	}
	if l.inv.Version != 2 {
		return
	}
	existing := make(map[[2]string]bool, len(l.gaps))
	for _, gap := range l.gaps {
		existing[[2]string{gap.ItemKey, gap.Kind}] = true
	}
	for _, member := range l.inv.Membership.Entries {
		l.members[member.DocumentKey] = member
		if member.Disposition != "active" && !existing[[2]string{member.DocumentKey, member.Disposition}] {
			l.addGap("build", member.DocumentKey, member.Disposition, "")
		}
	}
}
