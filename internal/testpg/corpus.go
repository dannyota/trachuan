//go:build integration

package testpg

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
)

// DropCorpusDatabase drops a database only if its marker names the test run.
func DropCorpusDatabase(t *testing.T, conn *pgx.Conn, name, runKey string) bool {
	t.Helper()
	ctx := context.Background()
	if !validRunKey(runKey) {
		t.Logf("leave database %s: invalid test run key", name)
		return false
	}
	var raw *string
	err := conn.QueryRow(ctx, `SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname=$1`, name).Scan(&raw)
	if err != nil {
		if err != pgx.ErrNoRows {
			t.Errorf("read database marker %s: %v", name, err)
		}
		return false
	}
	var marker struct {
		RunKey string `json:"run_key"`
	}
	if raw == nil || json.Unmarshal([]byte(*raw), &marker) != nil || marker.RunKey != runKey {
		t.Logf("leave database %s: ownership marker mismatch", name)
		return false
	}
	if _, err = conn.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		t.Errorf("drop database %s: %v", name, err)
		return false
	}
	return true
}
