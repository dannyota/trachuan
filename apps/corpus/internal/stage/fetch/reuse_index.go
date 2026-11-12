package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

type fileIdentity struct {
	name string
	size int64
}
type originIdentity struct {
	path, hash, name string
	size             int64
}
type baselineIndex struct {
	files   map[fileIdentity][]file
	origins map[originIdentity][]*store.RunFetchObservation
	tree    *store.RunFetchObservation
	oldest  time.Time
	hasTime bool
	unknown bool
}

func (b *baseline) indexOrigins(ctx context.Context) error {
	idx := &baselineIndex{files: make(map[fileIdentity][]file), origins: make(map[originIdentity][]*store.RunFetchObservation)}
	treeHash := digest(b.tree)
	for i := range b.observations {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("index baseline observations: %w", err)
		}
		o := &b.observations[i]
		if o.Path == "tree.json" && o.ContentSha256 == treeHash && idx.tree == nil {
			idx.tree = o
		}
		var key [3]json.RawMessage
		var name string
		if json.Unmarshal([]byte(o.ResourceKey), &key) != nil || json.Unmarshal(key[1], &name) != nil {
			continue
		}
		k := originIdentity{path: o.Path, hash: o.ContentSha256, size: o.Size, name: name}
		idx.origins[k] = append(idx.origins[k], o)
	}
	record := func(o *store.RunFetchObservation) {
		if o == nil || o.FetchedAt == nil {
			idx.unknown = true
			return
		}
		if !idx.hasTime || o.FetchedAt.Before(idx.oldest) {
			idx.oldest = *o.FetchedAt
			idx.hasTime = true
		}
	}
	record(idx.tree)
	seen := make(map[originIdentity]bool)
	for _, f := range b.files {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("index baseline files: %w", err)
		}
		k := fileIdentity{name: f.Name, size: f.Size}
		idx.files[k] = append(idx.files[k], f)
		origin := originIdentity{path: "files/" + f.SHA256, hash: f.SHA256, size: f.Size, name: f.Name}
		if seen[origin] {
			continue
		}
		seen[origin] = true
		observations := idx.origins[origin]
		if len(observations) == 0 {
			idx.unknown = true
		}
		for _, o := range observations {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("index baseline origins: %w", err)
			}
			record(o)
		}
	}
	b.index = idx
	return nil
}

func (b *baseline) originsFor(f file) []*store.RunFetchObservation {
	return b.index.origins[originIdentity{path: "files/" + f.SHA256, hash: f.SHA256, size: f.Size, name: f.Name}]
}
