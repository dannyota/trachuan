package testdb

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"

	"danny.vn/trachuan/internal/pgdrop"
	"github.com/jackc/pgx/v5"
)

func pruneSkipReason(name, current string, sessions int) string {
	if !regexp.MustCompile(`^corpus_tpl_[0-9a-f]{12}$`).MatchString(name) {
		return "invalid name"
	}
	if name == current {
		return "current migration"
	}
	if sessions != 0 {
		return "connected sessions"
	}
	return ""
}

func Prune(ctx context.Context, out io.Writer) error {
	current, err := migrationTemplateName()
	if err != nil {
		return err
	}
	adminURL := os.Getenv("CORPUS_TEST_ADMIN_URL")
	if adminURL == "" {
		adminURL = "postgres://trachuan@127.0.0.1:25433/postgres"
	}
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("connect to test database administrator")
	}
	defer func() { _ = admin.Close(ctx) }()
	if err = refuseActiveTests(ctx, admin); err != nil {
		return err
	}
	rows, err := admin.Query(ctx, `SELECT datname FROM pg_database WHERE left(datname, 11) = 'corpus_tpl_' ORDER BY datname`)
	if err != nil {
		return fmt.Errorf("list templates: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("read template name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list templates: %w", err)
	}
	rows.Close()
	for _, name := range names {
		if err := pruneOne(ctx, admin, out, name, current); err != nil {
			return err
		}
	}
	return nil
}

func refuseActiveTests(ctx context.Context, admin *pgx.Conn) error {
	var active bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE left(datname, 12) = 'corpus_test_' OR left(datname, 11) = 'corpus_tpl_')`).Scan(&active); err != nil {
		return fmt.Errorf("check test database sessions: %w", err)
	}
	if active {
		return fmt.Errorf("refusing to prune: test database sessions are connected")
	}
	return nil
}

func pruneOne(ctx context.Context, admin *pgx.Conn, out io.Writer, name, current string) error {
	if reason := pruneSkipReason(name, current, 0); reason != "" {
		_, err := fmt.Fprintf(out, "skipped %s: %s\n", name, reason)
		return err
	}
	if _, err := admin.Exec(ctx, `SELECT pg_advisory_lock(723548171, hashtext($1))`, name); err != nil {
		return fmt.Errorf("lock template: %w", err)
	}
	defer func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(723548171, hashtext($1))`, name)
	}()
	var template bool
	var sessions int
	err := admin.QueryRow(ctx, `SELECT datistemplate, (SELECT count(*) FROM pg_stat_activity WHERE datname = $1) FROM pg_database WHERE datname = $1`, name).Scan(&template, &sessions)
	if err == pgx.ErrNoRows {
		_, err = fmt.Fprintf(out, "skipped %s: no longer exists\n", name)
		return err
	}
	if err != nil {
		return fmt.Errorf("inspect template: %w", err)
	}
	if reason := pruneSkipReason(name, current, sessions); reason != "" {
		_, err = fmt.Fprintf(out, "skipped %s: %s\n", name, reason)
		return err
	}
	identifier := pgx.Identifier{name}.Sanitize()
	if template {
		if _, err = admin.Exec(ctx, "ALTER DATABASE "+identifier+" IS_TEMPLATE false"); err != nil {
			return fmt.Errorf("unmark template %s: %w", name, err)
		}
	}
	if err = pgdrop.DropOnly(ctx, admin, name); err != nil {
		if !template {
			return fmt.Errorf("drop incomplete template %s: %w", name, err)
		}
		if _, restoreErr := admin.Exec(context.WithoutCancel(ctx), "ALTER DATABASE "+identifier+" IS_TEMPLATE true"); restoreErr != nil {
			return fmt.Errorf("drop template %s: %w; restore template marker: %v", name, err, restoreErr)
		}
		return fmt.Errorf("drop template %s: %w", name, err)
	}
	_, err = fmt.Fprintf(out, "dropped %s\n", name)
	return err
}
