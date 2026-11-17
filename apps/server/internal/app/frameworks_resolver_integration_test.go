//go:build integration

package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/migrate"
	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/internal/embedding"
	frameworkserve "danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestFrameworkResolver_SeparatePointerAndReleaseFailures(t *testing.T) {
	engine, _, database := frameworkFixture(t, false, "")
	ctx := t.Context()
	authURL := testpg.NewDatabase(t)
	require.NoError(t, migrate.Up(ctx, authURL))
	auth, err := pgxpool.New(ctx, authURL)
	require.NoError(t, err)
	t.Cleanup(auth.Close)
	template := strings.Replace(database.String(), database.Path, "/{database}", 1)
	r := &release.Resolver{Auth: frameworksStore{auth}, Clock: time.Now, URLTemplate: template, Frameworks: &frameworkserve.Options{CursorKey: []byte(strings.Repeat("s", 32))}, EmbedderFor: func(string, embedding.Recipe) retrieval.Embedder { return frameworkFixtureQuery{} }}
	resolver := frameworksResolver{r}
	t.Cleanup(r.Close)
	_, err = resolver.ResolveFrameworks(ctx, "")
	require.ErrorIs(t, err, frameworkserve.ErrUnavailable)
	_, err = resolver.ResolveFrameworks(ctx, "20990101")
	require.ErrorIs(t, err, frameworkserve.ErrReleaseExpired)
	_, err = auth.Exec(ctx, `INSERT INTO corpus_release(country,active_db,activated_at,activated_by) VALUES('vn','corpus_vn_20990101',now(),'invented')`)
	require.NoError(t, err)
	_, err = auth.Exec(ctx, `INSERT INTO frameworks_release VALUES('vn',$1,now(),'invented')`, "frameworks_"+engine.Release())
	require.NoError(t, err)
	lease, err := resolver.ResolveFrameworks(ctx, "")
	require.NoError(t, err)
	status, err := lease.Engine.CorpusStatus(ctx, frameworkserve.StatusRequest{}, frameworkserve.Caller{})
	require.NoError(t, err)
	require.Equal(t, engine.Release(), status.Release)
	lease.Done()
	_, err = resolver.ResolveFrameworks(ctx, "expired")
	require.ErrorIs(t, err, frameworkserve.ErrReleaseExpired)
	_, err = auth.Exec(ctx, `UPDATE frameworks_release SET active_db='corpus_vn_20990101' WHERE country='vn'`)
	require.NoError(t, err)
	_, err = resolver.ResolveFrameworks(ctx, "")
	require.ErrorIs(t, err, frameworkserve.ErrUnavailable)
	var law string
	require.NoError(t, auth.QueryRow(ctx, `SELECT active_db FROM corpus_release WHERE country='vn'`).Scan(&law))
	require.Equal(t, "corpus_vn_20990101", law)
	auth.Close()
	_, err = resolver.ResolveFrameworks(context.Background(), "")
	require.ErrorIs(t, err, frameworkserve.ErrUnavailable)
}
