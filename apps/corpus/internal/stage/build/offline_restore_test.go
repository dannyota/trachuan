package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestoreDatabase_RejectsInvalidConfiguration(t *testing.T) {
	for _, cfg := range []Config{{}, {PgRestore: "pg_restore"}} {
		require.Error(t, RestoreDatabase(context.Background(), cfg, "bad-name", ""))
	}
	require.Error(t, WithTemporaryDatabase(context.Background(), Config{}, "invalid", "", nil, nil))
}

func TestRestoreDatabase_AcceptsFrameworkSchemaFunctions(t *testing.T) {
	script := filepath.Join(t.TempDir(), "pg_restore")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" = '-l' ]; then\n printf '%s\\n' '1; 0 0 FUNCTION public check_version_insert() builder'\nelse\n cat >/dev/null\nfi\n"), 0700))
	cfg := Config{BuilderURL: "postgres://builder@127.0.0.1/postgres?sslmode=disable", PgRestore: script}
	require.NoError(t, RestoreDatabase(context.Background(), cfg, "frameworks_20990101_eval", "invented.dump"))
}

func TestFrameworkRestore_AllowlistPreservesLawRejection(t *testing.T) {
	known := "1; 0 0 FUNCTION public check_version_insert() builder\n"
	_, err := filterFrameworksRestoreList(known)
	require.NoError(t, err)
	_, err = filterRestoreList(known)
	require.Error(t, err)
	for _, list := range []string{"1; 0 0 FUNCTION public unknown() builder\n", "1; 0 0 FUNCTION other check_version_insert() builder\n", "1; 0 0 PROCEDURE public check_version_insert() builder\n"} {
		_, err := filterFrameworksRestoreList(list)
		require.Error(t, err)
	}
}
