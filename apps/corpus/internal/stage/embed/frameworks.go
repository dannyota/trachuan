package embed

import (
	"context"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/internal/embedding"
)

func BuildUncached(ctx context.Context, client *embedding.Client, expected embedding.Recipe, file chunk.File, hash string) (Index, []byte, error) {
	return New(client, expected).buildCached(ctx, file, hash, nil)
}
