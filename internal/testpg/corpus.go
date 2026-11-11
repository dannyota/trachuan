//go:build integration

package testpg

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// DropCorpusDatabase drops a database only if its marker names the test run.
func DropCorpusDatabase(t *testing.T, conn *pgx.Conn, name, runKey string) bool {
	t.Helper()
	dropped, err := DropMarkedCorpus(context.Background(), conn, name, runKey)
	if err != nil {
		t.Errorf("drop marked corpus database: %v", err)
	}
	if !dropped && err == nil {
		t.Logf("leave database %s: ownership marker mismatch or invalid run key", name)
	}
	return dropped
}
