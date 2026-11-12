//go:build integration

package testpg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"danny.vn/trachuan/internal/pgdrop"
	"github.com/jackc/pgx/v5"
)

// OpenAdministrator connects only to a loopback PostgreSQL administrator.
func OpenAdministrator(ctx context.Context) (*pgx.Conn, *url.URL, error) {
	raw := os.Getenv("CORPUS_TEST_ADMIN_URL")
	if raw == "" {
		raw = "postgres://trachuan@127.0.0.1:25433/postgres"
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return nil, nil, errors.New("invalid test database URL")
	}
	for _, key := range []string{"host", "hostaddr", "dbname", "user", "password"} {
		if parsed.Query().Has(key) {
			return nil, nil, errors.New("test database URL must put connection identity in the URL authority and path")
		}
	}
	cfg, err := pgx.ParseConfig(raw)
	if err != nil {
		return nil, nil, errors.New("invalid test database configuration")
	}
	loopback := func(host string) bool {
		ip := net.ParseIP(host)
		return strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	}
	if !loopback(cfg.Host) {
		return nil, nil, errors.New("test database administrator must use a loopback host")
	}
	for _, fallback := range cfg.Fallbacks {
		if !loopback(fallback.Host) {
			return nil, nil, errors.New("test database administrator fallback must use a loopback host")
		}
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, nil, errors.New("connect to test database administrator")
	}
	return conn, parsed, nil
}

// CreateCorpus reserves and creates a marked test corpus database.
func CreateCorpus(ctx context.Context, conn *pgx.Conn, seed string) (string, string, error) {
	rel, _, err := acquireReservation(ctx, conn, seed)
	if err != nil {
		return "", "", fmt.Errorf("reserve test release: %w", err)
	}
	name, runKey := "corpus_vn_"+rel, "vn/"+rel
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", "", fmt.Errorf("create test corpus: %w", err)
	}
	marker := `{"run_key":"` + runKey + `"}`
	if _, err := conn.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '"+marker+"'"); err != nil {
		return name, runKey, fmt.Errorf("mark test corpus: %w", err)
	}
	return name, runKey, nil
}

// DropMarkedCorpus drops a corpus database only when its marker matches runKey.
func DropMarkedCorpus(ctx context.Context, conn *pgx.Conn, name, runKey string) (bool, error) {
	if !validRunKey(runKey) {
		return false, nil
	}
	var raw *string
	err := conn.QueryRow(ctx, `SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname=$1`, name).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read database marker: %w", err)
	}
	var marker struct {
		RunKey string `json:"run_key"`
	}
	if raw == nil || json.Unmarshal([]byte(*raw), &marker) != nil || marker.RunKey != runKey {
		return false, nil
	}
	if err := pgdrop.Drop(ctx, conn, name); err != nil {
		return false, fmt.Errorf("drop marked corpus: %w", err)
	}
	return true, nil
}
