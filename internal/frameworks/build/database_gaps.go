package build

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func loadGaps(ctx context.Context, tx pgx.Tx, d databaseInputs, ids databaseIDs) error {
	access := map[string]string{}
	for _, v := range d.versions {
		access[versionKey(v.Code, v.Version)] = v.Access
	}
	for _, gap := range d.gaps {
		key := versionKey(gap.Code, gap.Version)
		version, err := requiredID(ids.versions, key)
		if err != nil {
			return err
		}
		class, blocks, err := classifyGap(gap.Kind, access[key])
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) VALUES($1,$2,$3,$4,$5,$6)`, gap.Kind, class, version, gapSubject(gap), strconv.Itoa(gap.Count), blocks); err != nil {
			return hide("load quality gap", err)
		}
	}
	return nil
}
