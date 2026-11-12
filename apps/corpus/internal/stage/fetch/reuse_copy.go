package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/filecache"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

var errInvalidBaseline = errors.New("invalid baseline content")

func copyBaseline(src, dir, hash string, size int64) (vbpl.Downloaded, error) {
	in, err := os.Open(src)
	if errors.Is(err, os.ErrNotExist) {
		return vbpl.Downloaded{}, fmt.Errorf("%w: missing attachment", errInvalidBaseline)
	}
	if err != nil {
		return vbpl.Downloaded{}, fmt.Errorf("open baseline attachment: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp(dir, ".baseline-*")
	if err != nil {
		return vbpl.Downloaded{}, fmt.Errorf("create attachment: %w", err)
	}
	defer func() { _ = os.Remove(out.Name()) }()
	h := filecache.NewHasher()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, size+1))
	closeErr := out.Close()
	if err != nil {
		return vbpl.Downloaded{}, fmt.Errorf("copy baseline attachment: %w", err)
	}
	if closeErr != nil {
		return vbpl.Downloaded{}, fmt.Errorf("close attachment: %w", closeErr)
	}
	if n != size || h.Hashes().SHA256 != hash {
		return vbpl.Downloaded{}, fmt.Errorf("%w: attachment hash or size mismatch", errInvalidBaseline)
	}
	dest := filepath.Join(dir, hash)
	if err := os.Rename(out.Name(), dest); err != nil {
		return vbpl.Downloaded{}, fmt.Errorf("publish attachment: %w", err)
	}
	return vbpl.Downloaded{Path: dest, SHA256: hash, Size: n, Hashes: h.Hashes()}, nil
}

func (s *Stage) publishReuse(ctx context.Context, env stage.Env, r stage.Run, b *baseline, detail, diagram []byte, times [3]time.Time) (stage.Output, error) {
	observations := []stage.FetchObservation{}
	for _, p := range []struct {
		endpoint, path string
		data           []byte
		at             time.Time
	}{{"doc/detail", "detail.json", detail, times[0]}, {"doc/diagram", "diagram.json", diagram, times[2]}} {
		observations = append(observations, networkObservation(p.endpoint, p.path, p.data, p.at))
	}
	tree := b.index.tree
	if tree != nil {
		observations = append(observations, adopted(tree))
	} else {
		observations = append(observations, stage.FetchObservation{ResourceKey: `["doc/provision/tree",null,null]`, Source: "vbpl", Path: "tree.json", ContentSHA256: digest(b.tree), Size: int64(len(b.tree)), Acquisition: "adopted"})
	}
	art, err := env.Artifacts.PublishContent(ctx, s.Name(), s.Recipe(r.Snapshot), func(dir string) error {
		filesDir := filepath.Join(dir, "files")
		if e := os.Mkdir(filesDir, 0700); e != nil {
			return fmt.Errorf("create files directory: %w", e)
		}
		seen := map[string]bool{}
		seenFiles := map[originIdentity]bool{}
		for _, f := range b.files {
			if e := ctx.Err(); e != nil {
				return fmt.Errorf("copy baseline files: %w", e)
			}
			d, e := copyBaseline(filepath.Join(b.dir, "files", f.SHA256), filesDir, f.SHA256, f.Size)
			if e != nil {
				return e
			}
			var q hashStore
			if env.DB != nil {
				q = store.New(env.DB)
			}
			if e = writeHashes(ctx, q, d.Hashes); e != nil {
				return e
			}
			key := originIdentity{path: "files/" + f.SHA256, hash: f.SHA256, size: f.Size, name: f.Name}
			if seenFiles[key] {
				continue
			}
			seenFiles[key] = true
			origins := b.originsFor(f)
			if len(origins) == 0 {
				name := f.Name
				key, _ := json.Marshal([3]any{"attachment", name, nil})
				observations = append(observations, stage.FetchObservation{ResourceKey: string(key), Source: "vbpl", Name: &name, Path: "files/" + f.SHA256, ContentSHA256: f.SHA256, Size: f.Size, Acquisition: "adopted"})
			}
			for _, o := range origins {
				if e := ctx.Err(); e != nil {
					return fmt.Errorf("copy baseline observations: %w", e)
				}
				if !seen[o.ResourceKey] {
					observations = append(observations, adopted(o))
					seen[o.ResourceKey] = true
				}
			}
		}

		for _, p := range []struct {
			name string
			data []byte
		}{{"detail.json", detail}, {"diagram.json", diagram}, {"tree.json", b.tree}, {"files.json", b.filesJSON}, {"report.json", b.reportJSON}} {
			if e := os.WriteFile(filepath.Join(dir, p.name), p.data, 0600); e != nil {
				return fmt.Errorf("write %s: %w", p.name, e)
			}
		}
		return nil
	})
	if err != nil {
		return stage.Output{}, fmt.Errorf("publish reused fetch artifact: %w", err)
	}
	return stage.Output{Artifact: &art, State: "done", Observations: observations}, nil
}
