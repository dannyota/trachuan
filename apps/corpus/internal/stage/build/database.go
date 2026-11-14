package build

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"
	"time"

	"danny.vn/trachuan/internal/pgdrop"
	"github.com/jackc/pgx/v5"
)

// DatabaseName returns the final database name for a valid release identity.
func DatabaseName(cc, rel string) (string, error) {
	if !regexp.MustCompile(`^[a-z]{2}$`).MatchString(cc) || !regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(rel) {
		return "", fmt.Errorf("invalid corpus database identity")
	}
	return "corpus_" + cc + "_" + rel, nil
}

func lock(ctx context.Context, conn *pgx.Conn, cc, rel string) error {
	if _, err := DatabaseName(cc, rel); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(cc + "\x00" + rel))
	key := int64(binary.BigEndian.Uint64(sum[:8]))
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		return fmt.Errorf("acquire build lock: %w", err)
	}
	return nil
}

func unlock(ctx context.Context, conn *pgx.Conn, cc, rel string) error {
	sum := sha256.Sum256([]byte(cc + "\x00" + rel))
	key := int64(binary.BigEndian.Uint64(sum[:8]))
	var released bool
	if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&released); err != nil {
		return fmt.Errorf("release build lock: %w", err)
	}
	if !released {
		return fmt.Errorf("build lock not held")
	}
	return nil
}

func readMarker(ctx context.Context, admin *pgx.Conn, name string) (string, error) {
	var marker *string
	if err := admin.QueryRow(ctx, `SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname=$1`, name).Scan(&marker); err != nil {
		return "", fmt.Errorf("read database marker: %w", err)
	}
	if marker == nil {
		return "", nil
	}
	return *marker, nil
}

func recreateTemporary(ctx context.Context, admin *pgx.Conn, cfg Config, name, marker string) error {
	if !regexp.MustCompile(`^corpus_tmp_[a-z]{2}_[0-9a-f]{24}$`).MatchString(name) || cfg.Template == "" || marker == "" {
		return fmt.Errorf("invalid temporary database identity")
	}
	var exists, owned bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1), EXISTS(SELECT 1 FROM pg_database WHERE datname=$1 AND datdba=(SELECT oid FROM pg_roles WHERE rolname=current_user))`, name).Scan(&exists, &owned); err != nil {
		return fmt.Errorf("inspect temporary database: %w", err)
	}
	id := pgx.Identifier{name}.Sanitize()
	if exists {
		actual, err := readMarker(ctx, admin, name)
		if err != nil {
			return err
		}
		if !owned || !sameOwnership(actual, marker) {
			return fmt.Errorf("temporary database ownership mismatch")
		}
		if err = waitSessions(ctx, admin, name); err != nil {
			return err
		}
		if err = pgdrop.DropOnly(ctx, admin, name); err != nil {
			return fmt.Errorf("drop temporary database: %w", err)
		}
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+id+` TEMPLATE `+pgx.Identifier{cfg.Template}.Sanitize()); err != nil {
		return fmt.Errorf("create temporary database: %w", err)
	}
	if err := writeMarker(ctx, admin, name, marker); err != nil {
		return err
	}
	return nil
}

func renameToFinal(ctx context.Context, admin *pgx.Conn, temporary, final string) error {
	if !regexp.MustCompile(`^corpus_tmp_[a-z]{2}_[0-9a-f]{24}$`).MatchString(temporary) || !regexp.MustCompile(`^corpus_[a-z]{2}_[0-9]{8}(_[0-9]{1,2})?$`).MatchString(final) {
		return fmt.Errorf("invalid database name")
	}
	var foreign bool
	if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=$1 AND usesysid IS NOT NULL AND usename<>current_user)`, temporary).Scan(&foreign); err != nil {
		return fmt.Errorf("inspect sessions: %w", err)
	}
	if foreign {
		return fmt.Errorf("temporary database has foreign sessions")
	}
	if _, err := admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND usesysid IS NOT NULL AND usename=current_user AND pid<>pg_backend_pid()`, temporary); err != nil {
		return fmt.Errorf("disconnect builder sessions: %w", err)
	}
	if _, err := admin.Exec(ctx, `ALTER DATABASE `+pgx.Identifier{temporary}.Sanitize()+` RENAME TO `+pgx.Identifier{final}.Sanitize()); err != nil {
		return fmt.Errorf("rename database: %w", err)
	}
	return nil
}

func writeMarker(ctx context.Context, admin *pgx.Conn, name, marker string) error {
	var quoted string
	if err := admin.QueryRow(ctx, `SELECT quote_literal($1::text)`, marker).Scan(&quoted); err != nil {
		return fmt.Errorf("quote marker: %w", err)
	}
	if _, err := admin.Exec(ctx, `COMMENT ON DATABASE `+pgx.Identifier{name}.Sanitize()+` IS `+quoted); err != nil {
		return fmt.Errorf("mark temporary database: %w", err)
	}
	return nil
}

var sessionGrace = 5 * time.Second

func waitSessions(ctx context.Context, admin *pgx.Conn, name string) error {
	deadline := time.Now().Add(sessionGrace)
	for {
		var active bool
		if err := admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=$1 AND usesysid IS NOT NULL)`, name).Scan(&active); err != nil {
			return fmt.Errorf("inspect database sessions: %w", err)
		}
		if !active {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("temporary database has active sessions")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
