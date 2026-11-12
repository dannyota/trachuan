package fetch

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type hashStore interface {
	GetFileHash(context.Context, string) (store.FileHash, error)
	UpsertFileHash(context.Context, store.UpsertFileHashParams) error
}

func storedHashes(ctx context.Context, q hashStore, sha string) (filecache.Hashes, bool, error) {
	if q == nil {
		return filecache.Hashes{}, false, nil
	}
	row, err := q.GetFileHash(ctx, sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return filecache.Hashes{}, false, nil
	}
	if err != nil {
		return filecache.Hashes{}, false, fmt.Errorf("read file hashes: %w", err)
	}
	h := filecache.Hashes{SHA256: row.Sha256, MD5: row.Md5}
	if row.Etag16 != nil {
		h.ETag16 = *row.Etag16
	}
	if row.Etag5 != nil {
		h.ETag5 = *row.Etag5
	}
	return h, true, nil
}
func writeHashes(ctx context.Context, q hashStore, h filecache.Hashes) error {
	if q == nil {
		return nil
	}
	p := store.UpsertFileHashParams{Sha256: h.SHA256, Md5: h.MD5}
	if h.ETag16 != "" {
		p.Etag16 = &h.ETag16
	}
	if h.ETag5 != "" {
		p.Etag5 = &h.ETag5
	}
	if err := q.UpsertFileHash(ctx, p); err != nil {
		return fmt.Errorf("write file hashes: %w", err)
	}
	return nil
}
