//go:build integration

// Package testactivity reports database sessions in integration tests.
package testactivity

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Log reports the visible sessions connected to name.
func Log(t *testing.T, conn *pgx.Conn, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := conn.Query(ctx, `SELECT pid, usename, application_name, state, query_start FROM pg_stat_activity WHERE datname=$1 ORDER BY pid`, name)
	if err != nil {
		t.Logf("inspect sessions for %s: %v", name, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var pid int32
		var user, application, state *string
		var queryStart *time.Time
		if err := rows.Scan(&pid, &user, &application, &state, &queryStart); err != nil {
			t.Logf("read session for %s: %v", name, err)
			return
		}
		t.Logf("session database=%s pid=%d user=%s application=%s state=%s query_start=%v", name, pid, text(user), text(application), text(state), queryStart)
	}
	if err := rows.Err(); err != nil {
		t.Logf("read sessions for %s: %v", name, err)
	}
}

func text(value *string) string {
	if value == nil {
		return "<unavailable>"
	}
	return *value
}
