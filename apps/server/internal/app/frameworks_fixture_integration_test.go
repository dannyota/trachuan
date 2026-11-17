//go:build integration

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/build"
	frameworkserve "danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type frameworkFixtureDatabase struct {
	conn    *pgx.Conn
	changed bool
}

func (d frameworkFixtureDatabase) Build(ctx context.Context, r build.DatabaseRequest, load func(context.Context, *pgx.Conn) error, publish func() error) error {
	if err := load(ctx, d.conn); err != nil {
		return err
	}
	if err := extendFrameworkFixture(ctx, d.conn, d.changed); err != nil {
		return err
	}
	if err := os.WriteFile(r.DumpPath, []byte("invented fixture dump"), 0600); err != nil {
		return fmt.Errorf("write invented dump: %w", err)
	}
	return publish()
}

func (d frameworkFixtureDatabase) Evaluate(_ context.Context, _ build.EvaluationRequest, publish func(build.EvaluationResult) error) error {
	return publish(build.EvaluationResult{RetrievalRecipeStatus: "placeholder"})
}

func frameworkFixtureSources(t *testing.T, changed bool) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "source")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "trachuan"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sources"), 0700))
	t.Chdir(root)
	t.Setenv("FRAMEWORKS_SOURCE_DIR", source)
	t.Setenv("FRAMEWORKS_WORK_DIR", filepath.Join(root, "work"))
	t.Setenv("TMPDIR", root)
	var registry, titles strings.Builder
	registry.WriteString("frameworks:\n")
	titles.WriteString("code,version,citation,title\n")
	for i, code := range []string{"fw-alpha", "fw-beta", "fw-gamma"} {
		term, access := "quartz", "licensed"
		if i == 1 {
			access = "free"
		}
		if changed && i == 0 {
			term = "nebula"
		}
		data := []byte(fmt.Sprintf(`{"catalog":{"groups":[{"id":"a","title":"Invented family %s","controls":[{"id":"a-1","title":%q,"parts":[{"prose":%q}]},{"id":"a-2","title":"Invented second control","parts":[{"prose":"Invented quartz second safeguard."}]},{"id":"a-3","title":"Invented third control","parts":[{"prose":"Invented quartz third safeguard."}]}]}]}}`, code, "Invented "+term+" publisher title", strings.Repeat("Invented "+term+" safeguard. ", 20)))
		name := code + ".json"
		require.NoError(t, os.WriteFile(filepath.Join(source, name), data, 0600))
		//nolint:misspell // Registry field names follow the schema.
		_, err := fmt.Fprintf(&registry, "  - code: %s\n    version: '1'\n    name: Invented %s\n    publisher: Invented\n    language: en\n    access: %s\n    licence_kind: public-domain\n    licence_url: https://example.invalid/license\n    released: '2099-01-01'\n    ordinal: %d\n    aliases: ['%s standard']\n    citation_scheme: oscal-catalog\n    parser: nist-oscal-800-53\n    expected_controls: 4\n    files:\n      - path: %s\n        sha256: %x\n        size: %d\n        role: catalog\n        format: json\n", code, code, access, i+1, code, name, sha256.Sum256(data), len(data))
		require.NoError(t, err)
		for _, citation := range []string{"A", "A-1", "A-2", "A-3"} {
			_, err = fmt.Fprintf(&titles, "%s,1,%s,Invented quartz safeguard\n", code, citation)
			require.NoError(t, err)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "sources/frameworks.yaml"), []byte(registry.String()), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "trachuan/titles.csv"), []byte(titles.String()), 0600))
}

func frameworkFixture(t *testing.T, changed bool, logicalRelease string) (*frameworkserve.Engine, *pgxpool.Pool, *url.URL) {
	t.Helper()
	ctx := t.Context()
	builder := testpg.NewBuilder(t)
	admin, base, err := testpg.OpenAdministrator(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close(context.Background())) })
	reserved, runKey, err := testpg.CreateCorpus(ctx, admin, fmt.Sprintf("%s/%t", t.Name(), changed))
	require.NoError(t, err)
	name := strings.Replace(reserved, "corpus_vn_", "frameworks_", 1)
	require.True(t, testpg.DropCorpusDatabase(t, admin, reserved, runKey))
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{builder.Template}.Sanitize())
	require.NoError(t, err)
	_, err = admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '{\"run_key\":\""+runKey+"\"}'")
	require.NoError(t, err)
	t.Cleanup(func() { testpg.DropCorpusDatabase(t, admin, name, runKey) })
	base.Path = "/" + name
	conn, err := pgx.Connect(ctx, base.String())
	require.NoError(t, err)
	frameworkFixtureSources(t, changed)
	if logicalRelease == "" {
		logicalRelease = strings.TrimPrefix(name, "frameworks_")
	}
	var progress bytes.Buffer
	err = build.Run(ctx, logicalRelease, &progress, frameworkFixtureEmbedder{}, frameworkFixtureDatabase{conn: conn, changed: changed})
	require.NoError(t, conn.Close(ctx))
	require.NoError(t, err)
	require.NotContains(t, progress.String(), "quartz")
	require.NotContains(t, progress.String(), "nebula")
	pool, err := pgxpool.New(ctx, base.String())
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	engine, err := frameworkserve.Open(ctx, pool, frameworkFixtureQuery{}, frameworkserve.Options{CursorKey: []byte(strings.Repeat("s", 32))})
	require.NoError(t, err)
	return engine, pool, base
}

func extendFrameworkFixture(ctx context.Context, conn *pgx.Conn, changed bool) error {
	term := "quartz"
	if changed {
		term = "nebula"
	}
	_, err := conn.Exec(ctx, `UPDATE control_text SET notes=$1,tables=$2 WHERE control_id=(SELECT c.id FROM control c JOIN framework_version v ON v.id=c.version_id WHERE v.framework_code='fw-alpha' AND c.citation='A-1')`, []string{strings.Repeat("Invented "+term+" note. ", 2500)}, []string{"Invented " + term + " table."})
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,from_version_id,to_version_id,resolved)
 SELECT i,'fw-alpha','1','A-1','fw-beta','1','A-1','related',lpad(i::text,3,'0')||repeat('invented-source',10),1,2,true FROM generate_series(1,205) i`)
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `INSERT INTO mapping_evidence(mapping_id,evidence,licensed_version_ids) VALUES(1,$1,ARRAY[1]::bigint[])`, "Invented "+term+" private evidence.")
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) SELECT 'mapping_dropped','public',1,'A-1',i::text,false FROM generate_series(1,205) i`)
	if err != nil {
		return err
	}
	detail := "1"
	if changed {
		detail = "20"
	}
	_, err = conn.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) VALUES('text_incomplete','text',1,'A-1',$1,false)`, detail)
	return err
}
