package build

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
)

func WithDatabase(ctx context.Context, cfg Config, name, marker string, owns func(string) bool, use func(context.Context, *pgx.Conn) error) (result error) {
	if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(name) || cfg.BuilderURL == "" || cfg.Template == "" || marker == "" || owns == nil || !owns(marker) || use == nil {
		return fmt.Errorf("invalid offline database configuration")
	}
	adminCfg, err := pgx.ParseConfig(cfg.BuilderURL)
	if err != nil {
		return fmt.Errorf("parse builder connection: %w", err)
	}
	adminCfg.Database = "postgres"
	admin, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		return fmt.Errorf("connect builder administrator: %w", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	key := advisoryKey(name)
	if _, err = admin.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		return fmt.Errorf("lock offline database: %w", err)
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err = recreateOwned(workCtx, admin, cfg, name, marker, owns, sessionGrace); err != nil {
		return err
	}
	stop := watchLock(workCtx, admin, cancel, nil)
	defer func() { result = watchError(result, stop()) }()
	dbCfg := adminCfg.Copy()
	dbCfg.Database = name
	conn, err := pgx.ConnectConfig(workCtx, dbCfg)
	if err != nil {
		return fmt.Errorf("connect offline database: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if err = use(workCtx, conn); err != nil {
		return err
	}
	if err = stop(); err != nil {
		return err
	}
	if err = admin.Ping(ctx); err != nil {
		return fmt.Errorf("offline database lock lost: %w", err)
	}
	return nil
}

func advisoryKey(identity string) int64 {
	sum := sha256.Sum256([]byte(identity))
	return int64(binary.BigEndian.Uint64(sum[:8]))
}

func DumpDatabase(ctx context.Context, cfg Config, name, path string) error {
	return pgDump(ctx, cfg, name, path)
}
