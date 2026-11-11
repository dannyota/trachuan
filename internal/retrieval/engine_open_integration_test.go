//go:build integration

package retrieval

import (
	"context"
	"errors"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5/pgxpool"
)

type closeBeforeDimensions struct {
	fakeEmbedder
	pool *pgxpool.Pool
}

func (e *closeBeforeDimensions) Recipe(ctx context.Context) (embedding.Recipe, error) {
	// Open has consumed release metadata before it asks for the runtime recipe.
	e.pool.Close()
	return e.fakeEmbedder.Recipe(ctx)
}

func TestOpen_DatabaseDisappearsBetweenMetadataAndDimensions(t *testing.T) {
	db, _, _ := fixture(t)
	_, err := Open(context.Background(), db, &closeBeforeDimensions{pool: db}, fixtureOptions())
	if !errors.Is(err, ErrReleaseGone) || err.Error() != "read embedding dimensions failed" {
		t.Fatal("opening lost the disappearance between its queries", err)
	}
}
