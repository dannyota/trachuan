package fetch

import (
	"context"
	"encoding/json"
	"fmt"

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
	names   map[string][]file
	origins map[originIdentity][]*store.RunFetchObservation
	tree    *store.RunFetchObservation
}

func (b *baseline) indexOrigins(ctx context.Context) error {
	idx := &baselineIndex{files: make(map[fileIdentity][]file), names: make(map[string][]file), origins: make(map[originIdentity][]*store.RunFetchObservation)}
	treeHash := digest(b.tree)
	for i := range b.observations {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("index baseline observations: %w", err)
		}
		o := &b.observations[i]
		if o.Path == "tree.json" && o.ContentSha256 == treeHash && o.Size == int64(len(b.tree)) && idx.tree == nil {
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
	for _, f := range b.files {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("index baseline files: %w", err)
		}
		k := fileIdentity{name: f.Name, size: f.Size}
		idx.files[k] = append(idx.files[k], f)
		idx.names[f.Name] = append(idx.names[f.Name], f)
	}
	b.index = idx
	return nil
}

func (b *baseline) originsFor(f file) []*store.RunFetchObservation {
	return b.index.origins[originIdentity{path: "files/" + f.SHA256, hash: f.SHA256, size: f.Size, name: f.Name}]
}
