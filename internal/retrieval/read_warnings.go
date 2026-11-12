package retrieval

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func attachReadWarnings(ctx context.Context, tx pgx.Tx, nodes []Provision, units []Unit) error {
	byAnchor := make(map[string]int64, len(nodes))
	for _, p := range nodes {
		byAnchor[p.Identity.Anchor] = p.ID
	}
	var ids []int64
	seen := make(map[int64]bool)
	for _, unit := range units {
		id := byAnchor[unit.Anchor]
		if id != 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	warnings, err := loadWarnings(ctx, tx, ids)
	if err != nil {
		return err
	}
	for i := range units {
		set := warnings[byAnchor[units[i].Anchor]]
		units[i].Warnings, units[i].WarningsOmitted = set.warnings, set.omitted
	}
	return nil
}
