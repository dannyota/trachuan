//go:build integration

package testpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Release returns a valid test release derived from the test name.
func Release(t *testing.T) string {
	t.Helper()
	hash := sha256.Sum256([]byte(t.Name()))
	return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(binary.BigEndian.Uint32(hash[:4])%365)).Format("20060102")
}

// DropCorpusDatabase drops a database only if its marker names the test run.
func DropCorpusDatabase(t *testing.T, conn *pgx.Conn, name, runKey string) bool {
	t.Helper()
	ctx := context.Background()
	if len(runKey) != len("vn/20990101") || runKey[:7] != "vn/2099" {
		t.Logf("leave database %s: invalid test run key", name)
		return false
	}
	if _, err := time.Parse("20060102", runKey[3:]); err != nil {
		t.Logf("leave database %s: invalid test release", name)
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
