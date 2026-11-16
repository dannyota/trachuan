//go:build integration

package build

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/frameworks/relate"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func frameworkDB(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), testpg.NewDatabase(t, "vector"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close(context.Background())) })
	return conn
}

func databaseFixture(t *testing.T) (string, databaseInputs) {
	t.Helper()
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	base := filepath.Join(work, "20990101")
	d, err := readDatabaseInputs(base)
	require.NoError(t, err)
	d.edges = []relate.Edge{
		{From: relate.Key{Code: "fw-alpha", Version: "v1", Citation: "A-2"}, To: relate.Key{Code: "fw-beta", Version: "v1", Citation: "A-2"}, Relation: "related", Source: "invented", Resolved: true, Evidence: &relate.Evidence{Value: "Invented licensed evidence", Requires: []string{"fw:fw-alpha:v1"}}},
		{From: relate.Key{Code: "fw-alpha", Version: "v1", Citation: "A-2"}, To: relate.Key{Code: "fw-beta", Version: "v2", Citation: "B-9"}, Relation: "related", Source: "invented", Resolved: false},
	}
	return base, d
}

func TestDatabase_LoadAndDeterminism(t *testing.T) {
	base, d := databaseFixture(t)
	ctx := context.Background()
	var before []byte
	for range 2 {
		conn := frameworkDB(t)
		require.NoError(t, loadDatabase(ctx, conn, base, "20990101", d, fakeEmbedder{}))
		exported, err := OrderedExport(ctx, conn)
		require.NoError(t, err)
		if before != nil {
			require.True(t, bytes.Equal(before, exported), "ordered database exports differ")
		}
		before = exported
		var tables map[string][]map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(exported, &tables))
		require.Len(t, tables["release"], 1)
		release := tables["release"][0]
		require.Len(t, release, 21)
		require.NotContains(t, release, "step_recipes")
		require.NotContains(t, release, "built_at")
		var sealed bool
		var frameworks, versions, controls int
		require.NoError(t, conn.QueryRow(ctx, `SELECT versions_sealed,framework_count,version_count,control_count FROM release`).Scan(&sealed, &frameworks, &versions, &controls))
		require.True(t, sealed)
		require.Equal(t, []int{2, 2, 6}, []int{frameworks, versions, controls})
		var deps []int64
		require.NoError(t, conn.QueryRow(ctx, `SELECT licensed_version_ids FROM mapping_evidence`).Scan(&deps))
		require.Equal(t, []int64{1}, deps)
		var blocks int
		require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM quality_gap WHERE kind='title_missing' AND blocks AND class='public'`).Scan(&blocks))
		require.Equal(t, 2, blocks)
		var textRows, publicRows, sourceRows int
		require.NoError(t, conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM control_text),(SELECT count(*) FROM public_chunk),(SELECT count(*) FROM source_file)`).Scan(&textRows, &publicRows, &sourceRows))
		require.Equal(t, []int{6, 6, 2}, []int{textRows, publicRows, sourceRows})
		var publicLeaked bool
		require.NoError(t, conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public_posting WHERE version_id=1 AND token IN ('private','publisher','body'))`).Scan(&publicLeaked))
		require.False(t, publicLeaked)
		for step, provenance := range d.steps {
			var stored recipe
			var hash string
			require.NoError(t, conn.QueryRow(ctx, `SELECT `+pgx.Identifier{step + "_recipe"}.Sanitize()+`,`+pgx.Identifier{step + "_input_hash"}.Sanitize()+` FROM release`).Scan(&stored, &hash))
			a, _ := json.Marshal(provenance.Recipe)
			b, _ := json.Marshal(stored)
			require.True(t, equalJSON(a, b), "step recipe mismatch")
			require.Equal(t, provenance.InputHash, hash)
			require.True(t, equalJSON(a, release[step+"_recipe"]), "exported step recipe mismatch")
			var exportedHash string
			require.NoError(t, json.Unmarshal(release[step+"_input_hash"], &exportedHash))
			require.Equal(t, provenance.InputHash, exportedHash)
		}
	}
}

func TestDatabase_EvidenceMismatchRollsBack(t *testing.T) {
	base, d := databaseFixture(t)
	d.edges[0].Evidence.Requires = nil
	conn := frameworkDB(t)
	err := loadDatabase(context.Background(), conn, base, "20990101", d, fakeEmbedder{})
	require.ErrorContains(t, err, "evidence dependency mismatch")
	var controls int
	require.NoError(t, conn.QueryRow(context.Background(), `SELECT count(*) FROM control`).Scan(&controls))
	require.Zero(t, controls)
}
