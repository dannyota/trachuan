package build

import (
	"context"
	"fmt"

	"danny.vn/trachuan/internal/frameworks/relate"
	"github.com/jackc/pgx/v5"
)

func loadControls(ctx context.Context, tx pgx.Tx, d databaseInputs, ids *databaseIDs) error {
	for i, c := range d.controls {
		key := relate.Key{Code: c.Code, Version: c.Version, Citation: c.Citation}
		if _, ok := ids.controls[key]; ok {
			return fmt.Errorf("duplicate control identity")
		}
		ids.controls[key] = int64(i + 1)
	}
	for _, c := range d.controls {
		version, err := requiredID(ids.versions, versionKey(c.Code, c.Version))
		if err != nil {
			return err
		}
		id := ids.controls[relate.Key{Code: c.Code, Version: c.Version, Citation: c.Citation}]
		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		_, err = tx.Exec(ctx, `INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, version, c.Ordinal+1, c.Citation, c.CitationNorm, c.Kind, title, c.Status)
		if err != nil {
			return hide("load control", err)
		}
		notes, tables := c.Notes, c.Tables
		if notes == nil {
			notes = []string{}
		}
		if tables == nil {
			tables = []string{}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO control_text(control_id,title_original,body,notes,tables) VALUES($1,$2,$3,$4,$5)`, id, c.TitleOriginal, c.Body, notes, tables); err != nil {
			return hide("load control text", err)
		}
	}
	for _, c := range d.controls {
		values := make([]any, 3)
		for i, citation := range []*string{c.Parent, c.Supersedes, c.Amends} {
			if citation == nil {
				continue
			}
			id, err := requiredID(ids.controls, relate.Key{Code: c.Code, Version: c.Version, Citation: *citation})
			if err != nil {
				return err
			}
			values[i] = id
		}
		id := ids.controls[relate.Key{Code: c.Code, Version: c.Version, Citation: c.Citation}]
		if _, err := tx.Exec(ctx, `UPDATE control SET parent_id=$2,supersedes_id=$3,amends_id=$4 WHERE id=$1`, id, values[0], values[1], values[2]); err != nil {
			return hide("load control lineage", err)
		}
	}
	return nil
}
