package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrameworkDatabaseMarker_OnlySameWorkAndRelease(t *testing.T) {
	want := frameworkDatabaseMarker{WorkDir: "/private/work", Rel: "20990101", RunID: "new-run"}
	for _, tc := range []struct {
		marker frameworkDatabaseMarker
		owned  bool
	}{
		{frameworkDatabaseMarker{WorkDir: "/private/work", Rel: "20990101", RunID: "old-run"}, true},
		{frameworkDatabaseMarker{WorkDir: "/another/work", Rel: "20990101", RunID: "old-run"}, false},
		{frameworkDatabaseMarker{WorkDir: "/private/work", Rel: "20990102", RunID: "old-run"}, false},
		{frameworkDatabaseMarker{WorkDir: "/private/work", Rel: "20990101"}, false},
	} {
		raw, err := json.Marshal(tc.marker)
		require.NoError(t, err)
		require.Equal(t, tc.owned, want.owns(string(raw)))
	}
	for _, raw := range []string{"", "unmarked", "null", "{}", `{"work_dir":"/private/work","rel":"20990101","run_id":"old","unexpected":true}`} {
		require.False(t, want.owns(raw))
	}
}

func TestFrameworkDatabase_RequiresLocalConnection(t *testing.T) {
	for _, address := range []string{"postgres://builder@127.0.0.1/postgres", "postgres://builder@[::1]/postgres", "postgres://builder@localhost/postgres", "postgres://builder@/postgres?host=/private/tmp"} {
		require.NoError(t, localFrameworkDatabase(address))
	}
	for _, address := range []string{"postgres://builder@example.invalid/postgres", "postgres://builder@192.0.2.1/postgres", "postgres://builder@127.0.0.1,192.0.2.1/postgres", "postgres://builder@127.0.0.1/postgres?port=invalid"} {
		require.Error(t, localFrameworkDatabase(address))
	}
}
