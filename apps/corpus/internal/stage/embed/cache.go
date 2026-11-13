package embed

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type cachedRow struct {
	bytes []byte
	count int
}
type cachedArtifact struct {
	index   Index
	vectors []byte
	err     error
}

type cacheReader interface {
	LookupEmbedRows(context.Context, store.LookupEmbedRowsParams) ([]store.LookupEmbedRowsRow, error)
}

func (s *Stage) lookup(ctx context.Context, env stage.Env, run stage.Run, item stage.Item, file chunk.File) (map[string]cachedRow, error) {
	return s.lookupFrom(ctx, store.New(env.DB), env, run, item, file)
}

func (s *Stage) lookupFrom(ctx context.Context, q cacheReader, env stage.Env, run stage.Run, item stage.Item, file chunk.File) (map[string]cachedRow, error) {
	hashes := make([]string, 0, len(file.Chunks))
	for _, c := range file.Chunks {
		hashes = append(hashes, hashText(strings.TrimSpace(c.ContextPrefix)+"\n"+strings.TrimSpace(c.Content)))
	}
	rows, err := q.LookupEmbedRows(ctx, store.LookupEmbedRowsParams{RecipeHash: s.Recipe(run.Snapshot), InputHashes: hashes})
	if err != nil {
		return nil, fmt.Errorf("lookup embed rows: %w", err)
	}
	groups := make(map[int64][]store.LookupEmbedRowsRow)
	for _, r := range rows {
		groups[r.ArtifactID] = append(groups[r.ArtifactID], r)
	}
	hits := make(map[string]cachedRow)
	for _, refs := range groups {
		a := s.openCached(env, run, refs[0])
		for _, r := range refs {
			reason := a.err
			if reason == nil && (r.Row < 0 || int(r.Row) >= len(a.index.Rows)) {
				reason = fmt.Errorf("row out of range")
			}
			if reason == nil && (a.index.Rows[r.Row].InputSHA256 != r.InputSha256 || a.index.Rows[r.Row].TokenCount != int(r.TokenCount)) {
				reason = fmt.Errorf("index row mismatch")
			}
			if reason != nil {
				slog.Warn("embed cache miss", "run", run.Country+"/"+run.Rel, "key", item.Key, "reason", reason)
				continue
			}
			hits[r.InputSha256] = copyCachedRow(a, r)
		}
	}

	return hits, nil
}

func copyCachedRow(a cachedArtifact, row store.LookupEmbedRowsRow) cachedRow {
	width := a.index.Dimensions * 4
	start := int(row.Row) * width
	data := make([]byte, width)
	copy(data, a.vectors[start:start+width])
	return cachedRow{bytes: data, count: int(row.TokenCount)}
}

func (s *Stage) openCached(env stage.Env, run stage.Run, row store.LookupEmbedRowsRow) cachedArtifact {
	art, ok, err := env.Artifacts.OpenRecorded("embed", row.InputHash, s.Recipe(run.Snapshot), row.Sha256)
	if err != nil {
		return cachedArtifact{err: err}
	}
	if !ok {
		return cachedArtifact{err: fmt.Errorf("artifact missing or verification failed")}
	}
	if filepath.Clean(env.Artifacts.Abs(row.Path)) != filepath.Clean(env.Artifacts.Abs(art.Path)) {
		return cachedArtifact{err: fmt.Errorf("artifact path mismatch")}
	}
	dir := env.Artifacts.Abs(art.Path)
	idx, err := readVerifiedIndex(dir)
	if err != nil {
		return cachedArtifact{err: err}
	}
	if idx.Recipe != s.expected {
		return cachedArtifact{err: fmt.Errorf("artifact recipe mismatch")}
	}
	data, err := os.ReadFile(filepath.Join(dir, "vectors.f32"))
	if err != nil {
		return cachedArtifact{err: fmt.Errorf("read vectors: %w", err)}
	}
	if err = validateVectors(data, len(idx.Rows), idx.Dimensions); err != nil {
		return cachedArtifact{err: err}
	}
	return cachedArtifact{index: idx, vectors: data}
}
