//go:build integration

package serve

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type fixtureDatabase struct {
	conn              *pgx.Conn
	changed, extended bool
}

func (d fixtureDatabase) Build(ctx context.Context, r build.DatabaseRequest, load func(context.Context, *pgx.Conn) error, publish func() error) error {
	if err := load(ctx, d.conn); err != nil {
		return err
	}
	if d.extended {
		if err := extendOperationFixture(ctx, d.conn, d.changed); err != nil {
			return err
		}
	}
	if err := os.WriteFile(r.DumpPath, []byte("invented database dump"), 0600); err != nil {
		return fmt.Errorf("write invented dump: %w", err)
	}
	return publish()
}

func fixtureSources(t *testing.T, root string, changed bool) {
	t.Helper()
	source, work := filepath.Join(root, "source"), filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "trachuan"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sources"), 0700))
	t.Chdir(root)
	t.Setenv("FRAMEWORKS_SOURCE_DIR", source)
	t.Setenv("FRAMEWORKS_WORK_DIR", work)
	t.Setenv("TMPDIR", root)
	var registry, titles strings.Builder
	registry.WriteString("frameworks:\n")
	titles.WriteString("code,version,citation,title\n")
	for i, v := range []struct{ code, version, access string }{
		{"fw-alpha", "1", "licensed"}, {"fw-alpha", "2", "licensed"},
		{"fw-beta", "1", "free"}, {"fw-gamma", "1", "licensed"},
	} {
		term, repeats := "quartz", 20
		if changed && v.code == "fw-alpha" && v.version == "2" {
			term, repeats = "nebula", 800
		}
		body := strings.Repeat("Invented "+term+" assurance. ", repeats)
		data := []byte(fmt.Sprintf(`{"catalog":{"groups":[{"id":"a","title":%q,"controls":[{"id":"a-1","title":%q,"parts":[{"prose":%q}]},{"id":"a-2","title":"Invented quartz title two","parts":[{"prose":"Invented quartz second safeguard."}]},{"id":"a-3","title":"Invented quartz title three","parts":[{"prose":"Invented quartz third safeguard."}]}]}]}}`, "Invented family "+v.code+" "+v.version, "Invented "+term+" publisher title", body))
		name := v.code + "-" + v.version + ".json"
		require.NoError(t, os.WriteFile(filepath.Join(source, name), data, 0600))
		//nolint:misspell // Registry field names follow the schema.
		_, err := fmt.Fprintf(&registry, "  - code: %s\n    version: '%s'\n    name: Invented %s\n    publisher: Invented\n    language: en\n    access: %s\n    licence_kind: public-domain\n    licence_url: https://example.invalid/license\n    released: '2099-01-%02d'\n    ordinal: %d\n    aliases: ['%s standard']\n    citation_scheme: oscal-catalog\n    parser: nist-oscal-800-53\n    expected_controls: 4\n    files:\n      - path: %s\n        sha256: %x\n        size: %d\n        role: catalog\n        format: json\n", v.code, v.version, v.code, v.access, i+1, i+1, v.code, name, sha256.Sum256(data), len(data))
		require.NoError(t, err)
		for _, citation := range []string{"A", "A-1", "A-2", "A-3"} {
			_, err = fmt.Fprintf(&titles, "%s,%s,%s,Invented quartz safeguard\n", v.code, v.version, citation)
			require.NoError(t, err)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "sources/frameworks.yaml"), []byte(registry.String()), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "trachuan/titles.csv"), []byte(titles.String()), 0600))
}

func buildFixture(t *testing.T, changed bool) (*Engine, *queryTrace) {
	t.Helper()
	return buildFixtureMode(t, changed, false)
}

func buildOperationsFixture(t *testing.T, changed bool) (*Engine, *queryTrace) {
	t.Helper()
	return buildFixtureMode(t, changed, true)
}

func buildFixtureMode(t *testing.T, changed, extended bool) (*Engine, *queryTrace) {
	t.Helper()
	return buildFixtureWithCases(t, changed, extended, false)
}

func buildFixtureWithCases(t *testing.T, changed, extended, calibrated bool) (*Engine, *queryTrace) {
	t.Helper()
	ctx := context.Background()
	template := testpg.NewBuilder(t)
	admin, base, err := testpg.OpenAdministrator(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close(ctx)) })
	name, runKey, err := testpg.CreateCorpus(ctx, admin, fmt.Sprintf("%s/%t", t.Name(), changed))
	require.NoError(t, err)
	frameworkName := strings.Replace(name, "corpus_vn_", "frameworks_", 1)
	require.True(t, testpg.DropCorpusDatabase(t, admin, name, runKey))
	require.NoError(t, execFixture(ctx, admin, "CREATE DATABASE "+pgx.Identifier{frameworkName}.Sanitize()+" TEMPLATE "+pgx.Identifier{template.Template}.Sanitize()))
	require.NoError(t, execFixture(ctx, admin, "COMMENT ON DATABASE "+pgx.Identifier{frameworkName}.Sanitize()+" IS '{\"run_key\":\""+runKey+"\"}'"))
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, frameworkName, runKey) })
	base.Path = "/" + frameworkName
	conn, err := pgx.Connect(ctx, base.String())
	require.NoError(t, err)
	root := t.TempDir()
	fixtureSources(t, root, changed)
	if calibrated {
		fixtureGoldenCases(t, root)
	}
	var progress bytes.Buffer
	err = build.Run(ctx, "20990101", &progress, syntheticEmbedder{}, fixtureDatabase{conn: conn, changed: changed, extended: extended}, openFixtureCalibration)
	var classes map[string]relationClass
	if err == nil {
		classes = readRelationClasses(t, conn)
	}
	require.NoError(t, conn.Close(ctx))
	require.NoError(t, err)
	require.NotContains(t, progress.String(), "quartz")
	require.NotContains(t, progress.String(), "nebula")
	config, err := pgxpool.ParseConfig(base.String())
	require.NoError(t, err)
	trace := &queryTrace{classes: classes}
	config.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	engine, err := Open(ctx, pool, syntheticQuery{}, Options{CursorKey: []byte(strings.Repeat("s", 32))})
	require.NoError(t, err)
	trace.assertProjection(t, nil)
	trace.reset()
	return engine, trace
}

func execFixture(ctx context.Context, conn *pgx.Conn, sql string) error {
	_, err := conn.Exec(ctx, sql)
	return err
}
