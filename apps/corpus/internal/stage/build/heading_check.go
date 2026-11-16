package build

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"github.com/jackc/pgx/v5"
)

type headingChecker interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func CheckHeadingExclusions(ctx context.Context, db headingChecker, entries []source.HeadingExclusion) error {
	for _, e := range entries {
		var found bool
		err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM document d JOIN provision p ON p.document_id=d.id WHERE d.key=$1 AND p.anchor=$2)`, e.Key, e.Anchor).Scan(&found)
		if err != nil {
			return fmt.Errorf("check heading exclusion: %w", err)
		}
		if !found {
			return fmt.Errorf("heading exclusion %s/%s absent from release", e.Key, e.Anchor)
		}
	}
	return nil
}
