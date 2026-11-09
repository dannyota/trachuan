package build

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// InspectVersions reads the versions that must be frozen into the recipe.
func InspectVersions(ctx context.Context, cfg Config) (Versions, error) {
	c, err := pgx.ParseConfig(cfg.BuilderURL)
	if err != nil {
		return Versions{}, fmt.Errorf("parse builder configuration: %w", err)
	}
	c.Database = cfg.Template
	db, err := pgx.ConnectConfig(ctx, c)
	if err != nil {
		return Versions{}, fmt.Errorf("connect template: %w", err)
	}
	defer func() { _ = db.Close(context.Background()) }()
	return inspectVersions(ctx, cfg, db)
}

func inspectVersions(ctx context.Context, cfg Config, db *pgx.Conn) (Versions, error) {
	var v Versions
	var number int
	if err := db.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&number); err != nil {
		return v, fmt.Errorf("read PostgreSQL version: %w", err)
	}
	v.PostgreSQLMajor = strconv.Itoa(number / 10000)
	if err := db.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname='vector'`).Scan(&v.PGVector); err != nil {
		return v, fmt.Errorf("read vector version: %w", err)
	}
	if err := db.QueryRow(ctx, `SELECT extversion FROM pg_extension WHERE extname='ltree'`).Scan(&v.Ltree); err != nil {
		return v, fmt.Errorf("read ltree version: %w", err)
	}
	for _, tool := range []struct {
		path   string
		result *string
	}{{cfg.PgDump, &v.PgDump}, {cfg.PgRestore, &v.PgRestore}} {
		if tool.path == "" {
			return Versions{}, fmt.Errorf("missing PostgreSQL tool")
		}
		deadline, cancel := context.WithTimeout(ctx, 15*time.Second)
		output, err := exec.CommandContext(deadline, tool.path, "--version").Output()
		cancel()
		if err != nil {
			return Versions{}, fmt.Errorf("read PostgreSQL tool version: %w", err)
		}
		*tool.result = strings.TrimSpace(string(output))
		if *tool.result == "" {
			return Versions{}, fmt.Errorf("empty PostgreSQL tool version")
		}
	}
	return v, nil
}
