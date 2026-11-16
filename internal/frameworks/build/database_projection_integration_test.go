//go:build integration

package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func databasePublicProjection(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	var projection string
	err := conn.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'controls',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM control c),
 'chunks',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM public_chunk c),
 'postings',(SELECT jsonb_agg(to_jsonb(p) ORDER BY version_id,chunk_id,token) FROM public_posting p),
 'statistics',(SELECT to_jsonb(s) FROM public_lexical_stats s),
 'frequencies',(SELECT jsonb_agg(to_jsonb(s) ORDER BY token) FROM public_lexical_df s),
 'gaps',(SELECT jsonb_agg(to_jsonb(g) ORDER BY kind,version_id,subject,detail) FROM quality_gap g WHERE class='public')
 )::text`).Scan(&projection)
	require.NoError(t, err)
	return projection
}

func TestDatabase_LicensedRebuildPreservesPublicIndex(t *testing.T) {
	base, d := databaseFixture(t)
	ctx := context.Background()
	beforeDB := frameworkDB(t)
	require.NoError(t, loadDatabase(ctx, beforeDB, base, "20990101", d, fakeEmbedder{}))
	before := databasePublicProjection(t, beforeDB)
	source := filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "fw-alpha.json")
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	data = bytes.ReplaceAll(data, []byte("private body"), []byte(strings.Repeat("different licensed material ", 3)))
	data = bytes.ReplaceAll(data, []byte("publisher private title"), []byte("changed original title"))
	require.NoError(t, os.WriteFile(source, data, 0600))
	editRegistry(t, func(reg *registry.Registry) {
		reg.Frameworks[0].Files[0].SHA256 = digest(data)
		reg.Frameworks[0].Files[0].Size = int64(len(data))
	})
	require.NoError(t, testRun(ctx, "20990102", &bytes.Buffer{}))
	afterBase := filepath.Join(filepath.Dir(base), "20990102")
	after, err := readDatabaseInputs(afterBase)
	require.NoError(t, err)
	afterDB := frameworkDB(t)
	require.NoError(t, loadDatabase(ctx, afterDB, afterBase, "20990102", after, fakeEmbedder{}))
	require.Equal(t, before, databasePublicProjection(t, afterDB))
}

func TestDatabase_RejectedEmbeddingsAreGapsOnly(t *testing.T) {
	work := fixture(t)
	ctx := context.Background()
	e := &rejectingEmbedder{}
	require.NoError(t, Run(ctx, "20990101", &bytes.Buffer{}, e, fakeDatabase{}))
	base := filepath.Join(work, "20990101")
	d, err := readDatabaseInputs(base)
	require.NoError(t, err)
	conn := frameworkDB(t)
	require.NoError(t, loadDatabase(ctx, conn, base, "20990101", d, e))
	var inserted, rejected int
	require.NoError(t, conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM body_chunk)+(SELECT count(*) FROM public_chunk),coalesce((SELECT sum(detail::integer) FROM quality_gap WHERE kind IN ('embedding_input_too_long','embedding_input_bytes_exceeded')),0)`).Scan(&inserted, &rejected))
	require.Equal(t, len(d.chunks), inserted+rejected)
	require.Equal(t, 8, rejected)
}
