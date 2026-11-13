package pinned

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type Acquisition struct {
	Artifact     store.GetPinnedArtifactRow
	Observations []store.RunFetchObservation
}

func Acquire(ctx context.Context, q *store.Queries, artifacts artifact.Store, country string, pin source.Pin) (Acquisition, error) {
	row, err := Load(ctx, q, artifacts, country, pin, "fetch")
	if err != nil {
		return Acquisition{}, err
	}
	observations, err := q.ListItemFetchObservations(ctx, store.ListItemFetchObservationsParams{RunID: row.RunID, ItemKey: pin.Key, Generation: row.Generation, Attempt: row.Attempt})
	if err != nil {
		return Acquisition{}, fmt.Errorf("read pinned observations: %w", err)
	}
	return Acquisition{Artifact: row, Observations: observations}, nil
}

func Copy(ctx context.Context, from, to string) error {
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == "MANIFEST.sha256" {
			return nil
		}
		target := filepath.Join(to, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular pinned file %s", rel)
		}
		return copyFile(path, target)
	})
	if err != nil {
		return fmt.Errorf("copy pinned bundle: %w", err)
	}
	return nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
