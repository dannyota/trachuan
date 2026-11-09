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
		_, err := l.tx.Exec(ctx, `INSERT INTO coverage_gap(id,kind,document_id,document_key,stage,item_key,detail,message,note) VALUES($1,$2,$3,$4,$5,$6,$7,'','')`, l.counts.Gaps+1, g.Kind, docID(l.docs[g.ItemKey]), key, g.Stage, g.ItemKey, g.Detail)
		if err != nil {
			return fmt.Errorf("insert coverage gap: %w", err)
		}
		l.counts.Gaps++
		copy := g
		previous = &copy
	}
	return nil
}
