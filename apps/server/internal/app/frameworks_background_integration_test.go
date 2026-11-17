//go:build integration

package app

import (
	"context"
	"strings"
	"sync/atomic"
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

type blockedFrameworkRecipe struct {
	frameworkFixtureQuery
	entered chan<- struct{}
	proceed <-chan struct{}
}

func (q blockedFrameworkRecipe) Recipe(ctx context.Context) (embedding.Recipe, error) {
	q.entered <- struct{}{}
	select {
	case <-q.proceed:
		return q.frameworkFixtureQuery.Recipe(ctx)
	case <-ctx.Done():
		return embedding.Recipe{}, ctx.Err()
	}
}

func TestFrameworkResolver_ProductionOpenNeverBlocksRequests(t *testing.T) {
	first, _, database := frameworkFixture(t, false, "")
	second, _, _ := frameworkFixture(t, true, "")
	ctx := t.Context()
	authURL := testpg.NewDatabase(t)
	require.NoError(t, migrate.Up(ctx, authURL))
	auth, err := pgxpool.New(ctx, authURL)
	require.NoError(t, err)
	t.Cleanup(auth.Close)
	_, err = auth.Exec(ctx, `INSERT INTO frameworks_release VALUES('vn',$1,now(),'invented')`, "frameworks_"+first.Release())
	require.NoError(t, err)
	entered, proceed := make(chan struct{}, 2), make(chan struct{}, 2)
	var opens atomic.Int32
	r := &release.Resolver{
		Auth: frameworksStore{auth}, Clock: time.Now,
		URLTemplate: strings.Replace(database.String(), database.Path, "/{database}", 1),
		Frameworks:  &frameworkserve.Options{CursorKey: []byte(strings.Repeat("s", 32))},
		EmbedderFor: func(string, embedding.Recipe) retrieval.Embedder {
			opens.Add(1)
			return blockedFrameworkRecipe{entered: entered, proceed: proceed}
		},
	}
	resolver := frameworksResolver{r}
	probe, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = resolver.ResolveFrameworks(probe, "")
	cancel()
	require.ErrorIs(t, err, frameworkserve.ErrUnavailable)
	require.Zero(t, opens.Load())
	watchFrameworkResolver(t, r)
	for i, rel := range []string{first.Release(), second.Release()} {
		if i > 0 {
			_, err = auth.Exec(ctx, `UPDATE frameworks_release SET active_db=$1 WHERE country='vn'`, "frameworks_"+rel)
			require.NoError(t, err)
		}
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("watcher did not open the active release")
		}
		probe, stop := context.WithTimeout(ctx, 50*time.Millisecond)
		_, err = resolver.ResolveFrameworks(probe, rel)
		stop()
		require.ErrorIs(t, err, frameworkserve.ErrUnavailable)
		proceed <- struct{}{}
		awaitFrameworkResolver(t, resolver, rel)
	}
	require.Equal(t, int32(2), opens.Load())
}
