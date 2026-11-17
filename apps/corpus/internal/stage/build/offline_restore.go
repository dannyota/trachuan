package build

import (
	"context"
	"fmt"
	"regexp"

	"danny.vn/trachuan/internal/pgdrop"
	"github.com/jackc/pgx/v5"
)

func RestoreDatabase(ctx context.Context, cfg Config, name, path string) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(name) || cfg.PgRestore == "" || path == "" {
		return fmt.Errorf("invalid restore configuration")
	}
	return pgRestoreFiltered(ctx, cfg, name, path, filterFrameworksRestoreList)
}

func WithTemporaryDatabase(ctx context.Context, cfg Config, name, marker string, owns func(string) bool, use func(context.Context, *pgx.Conn) error) error {
	return withOfflineDatabase(ctx, cfg, name, marker, owns, true, use)
}

func dropOfflineOwned(ctx context.Context, admin *pgx.Conn, name, marker string, owns func(string) bool) error {
	actual, err := readMarker(ctx, admin, name)
	if err != nil {
		return err
	}
	var owned bool
	if err = admin.QueryRow(ctx, `SELECT datdba=(SELECT oid FROM pg_roles WHERE rolname=current_user) FROM pg_database WHERE datname=$1`, name).Scan(&owned); err != nil {
		return fmt.Errorf("inspect cleanup ownership: %w", err)
	}
	if !owned || actual != marker || !owns(actual) {
		return fmt.Errorf("temporary database cleanup ownership mismatch")
	}
	if err = waitSessions(ctx, admin, name, sessionGrace); err != nil {
		return err
	}
	if err = pgdrop.DropOnly(ctx, admin, name); err != nil {
		return fmt.Errorf("drop owned offline database: %w", err)
	}
	return nil
}

func filterFrameworksRestoreList(list string) (string, error) {
	functions := map[string]bool{}
	for _, name := range []string{"reject_version_mutation", "reject_lifecycle_reset", "check_version_insert", "check_evidence_insert", "check_versions_seal_transition", "check_evidence_dependencies"} {
		functions[name+"()"] = true
	}
	return filterRestoreListAllowing(list, functions)
}
