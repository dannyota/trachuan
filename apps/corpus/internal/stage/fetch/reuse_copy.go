package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
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
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, size+1))
	closeErr := out.Close()
	if err != nil {
		return vbpl.Downloaded{}, fmt.Errorf("copy baseline attachment: %w", err)
	}
	if closeErr != nil {
		return vbpl.Downloaded{}, fmt.Errorf("close attachment: %w", closeErr)
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != hash {
		return vbpl.Downloaded{}, fmt.Errorf("%w: attachment hash or size mismatch", errInvalidBaseline)
	}
	dest := filepath.Join(dir, hash)
	if err := os.Rename(out.Name(), dest); err != nil {
		return vbpl.Downloaded{}, fmt.Errorf("publish attachment: %w", err)
	}
	return vbpl.Downloaded{Path: dest, SHA256: hash, Size: n}, nil
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
	observations = append(observations, adopted(tree))
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
			if _, e := copyBaseline(filepath.Join(b.dir, "files", f.SHA256), filesDir, f.SHA256, f.Size); e != nil {
				return e
			}
			key := originIdentity{path: "files/" + f.SHA256, hash: f.SHA256, size: f.Size, name: f.Name}
			if seenFiles[key] {
				continue
			}
			seenFiles[key] = true
			for _, o := range b.originsFor(f) {
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

func (b *baseline) copyMatching(entry vbpl.File, dir string, now time.Time, limit time.Duration) (vbpl.Downloaded, *stage.FetchObservation, error) {
	if entry.UpdatedAt.IsZero() || entry.Size < 0 || entry.Size > 50_000_000 {
		return vbpl.Downloaded{}, nil, nil
	}
	files := b.index.files[fileIdentity{name: entry.Name, size: entry.Size}]
	if len(files) != 1 {
		return vbpl.Downloaded{}, nil, nil
	}
	f := files[0]
	origins := b.originsFor(f)
	if len(origins) == 0 {
		return vbpl.Downloaded{}, nil, nil
	}
	o := origins[0]
	if o.FetchedAt == nil || o.FetchedAt.Before(entry.UpdatedAt) || o.FetchedAt.Before(now.Add(-limit)) {
		return vbpl.Downloaded{}, nil, nil
	}
	d, err := copyBaseline(filepath.Join(b.dir, "files", f.SHA256), dir, f.SHA256, f.Size)
	if err != nil {
		return vbpl.Downloaded{}, nil, err
	}
	obs := adopted(o)
	return d, &obs, nil
}
