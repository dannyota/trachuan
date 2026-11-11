package checkpoint

import (
	"context"
	"fmt"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type workshopSnapshot interface {
	snapshotID() string
	artifactReferenced(context.Context, string, string) (bool, error)
	close(context.Context)
}

type databaseSnapshot struct {
	conn *pgx.Conn
	tx   pgx.Tx
	id   string
}

func openWorkshopSnapshot(ctx context.Context, databaseURL string) (workshopSnapshot, error) {
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect workshop snapshot: %w", err)
	}
	snapshot := &databaseSnapshot{conn: conn}
	snapshot.tx, err = conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		snapshot.close(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("begin workshop snapshot: %w", err)
	}
	if err = snapshot.tx.QueryRow(ctx, "SELECT pg_export_snapshot()").Scan(&snapshot.id); err != nil {
		snapshot.close(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("export workshop snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *databaseSnapshot) snapshotID() string { return s.id }

func (s *databaseSnapshot) artifactReferenced(ctx context.Context, root, rel string) (bool, error) {
	// Acquisition-only references remain hints; only items in the dump's
	// snapshot make corrupt bytes fatal, even after a concurrent retry detaches them.
	used, err := store.New(s.tx).ArtifactPathReferenced(ctx, store.ArtifactPathReferencedParams{RelativePath: rel, AbsolutePath: filepath.Join(root, rel)})
	if err != nil {
		return false, fmt.Errorf("query snapshot artifact reference: %w", err)
	}
	return used, nil
}

func (s *databaseSnapshot) close(ctx context.Context) {
	if s.tx != nil {
		_ = s.tx.Rollback(ctx)
	}
	_ = s.conn.Close(ctx)
}
