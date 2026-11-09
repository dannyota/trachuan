// Package checkpoint saves the workshop and immutable artifacts.
package checkpoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"danny.vn/traquydinh/apps/corpus/internal/artifact"
	"danny.vn/traquydinh/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// Config contains checkpoint paths and retention settings.
type Config struct {
	Dir, ArtifactsRoot, DatabaseURL, PgDump string
	Keep                                    int
}

// Dumper writes a consistent workshop dump.
type Dumper interface {
	Dump(ctx context.Context, databaseURL, path string) error
}

// Taker saves a stage boundary.
type Taker struct {
	Config     Config
	Dumper     Dumper
	Now        func() time.Time
	referenced func(context.Context, string, string) (bool, error)
}
type metadata struct {
	Stage      string    `json:"stage"`
	Generation int       `json:"generation"`
	Time       time.Time `json:"time"`
	DumpSHA256 string    `json:"dump_sha256"`
}

// Take dumps the workshop, copies new artifacts and records the checkpoint.
func (t Taker) Take(ctx context.Context, runKey string, generation int, stage string) (string, error) {
	if !safeName(runKey) || !safeName(stage) || generation < 1 {
		return "", errors.New("invalid checkpoint identity")
	}
	dir := filepath.Join(t.Config.Dir, runKey, fmt.Sprintf("%d-%s", generation, stage))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create checkpoint: %w", err)
	}
	if err := os.Remove(filepath.Join(dir, "checkpoint.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("remove old checkpoint marker: %w", err)
	}
	dumper := t.Dumper
	if dumper == nil {
		dumper = PgDump{Path: t.Config.PgDump}
	}
	dump := filepath.Join(dir, "workshop.dump")
	if err := dumper.Dump(ctx, t.Config.DatabaseURL, dump); err != nil {
		return "", fmt.Errorf("dump workshop: %w", err)
	}
	f, err := os.Open(dump)
	if err != nil {
		return "", fmt.Errorf("open dump: %w", err)
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return "", fmt.Errorf("hash dump: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close dump: %w", closeErr)
	}
	if copyErr := t.copyArtifacts(ctx, t.Config.ArtifactsRoot, filepath.Join(t.Config.Dir, "artifacts")); copyErr != nil {
		return "", copyErr
	}
	now := time.Now
	if t.Now != nil {
		now = t.Now
	}
	data, err := json.Marshal(metadata{Stage: stage, Generation: generation, Time: now().UTC(), DumpSHA256: fmt.Sprintf("%x", h.Sum(nil))})
	if err != nil {
		return "", fmt.Errorf("encode checkpoint: %w", err)
	}
	marker, err := os.CreateTemp(dir, ".checkpoint-")
	if err != nil {
		return "", fmt.Errorf("create checkpoint marker: %w", err)
	}
	defer func() { _ = os.Remove(marker.Name()) }()
	_, writeErr := marker.Write(data)
	closeErr = marker.Close()
	if writeErr != nil {
		return "", fmt.Errorf("write checkpoint: %w", writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close checkpoint: %w", closeErr)
	}
	if err := os.Rename(marker.Name(), filepath.Join(dir, "checkpoint.json")); err != nil {
		return "", fmt.Errorf("publish checkpoint: %w", err)
	}
	keep := t.Config.Keep
	if keep <= 0 {
		keep = 3
	}
	if err := prune(filepath.Dir(dir), keep); err != nil {
		return "", err
	}
	return dir, nil
}
func safeName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, `/\\`)
}

func (t Taker) copyArtifacts(ctx context.Context, root, dest string) error {
	if err := os.MkdirAll(dest, 0700); err != nil {
		return fmt.Errorf("create artifact destination: %w", err)
	}
	stages, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read artifacts: %w", err)
	}
	for _, stage := range stages {
		if !stage.IsDir() || stage.Name() == "" || strings.Trim(stage.Name(), "abcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
			continue
		}
		entries, readErr := os.ReadDir(filepath.Join(root, stage.Name()))
		if readErr != nil {
			return fmt.Errorf("read artifact stage: %w", readErr)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("copy artifacts: %w", err)
			}
			if !entry.IsDir() || !artifactName(entry.Name()) {
				continue
			}
			src := filepath.Join(root, stage.Name(), entry.Name())
			rel := filepath.Join(stage.Name(), entry.Name())
			target := filepath.Join(dest, stage.Name(), entry.Name())
			// Copies are verified when made; an existing copy is never re-hashed.
			if _, statErr := os.Lstat(target); statErr == nil {
				continue
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("inspect artifact copy: %w", statErr)
			}
			if _, verifyErr := artifact.Verify(src); verifyErr != nil {
				referenced := t.referenced
				if referenced == nil {
					referenced = t.artifactReferenced
				}
				used, queryErr := referenced(ctx, root, rel)
				if queryErr != nil {
					return fmt.Errorf("check artifact reference: %w", queryErr)
				}
				if used {
					return fmt.Errorf("verify referenced artifact %s: %w", src, verifyErr)
				}
				slog.Warn("skipping corrupt orphan artifact", "path", src, "error", verifyErr)
				continue
			}
			if err := copyArtifact(src, target); err != nil {
				return err
			}
		}
	}
	return nil
}
func artifactName(name string) bool {
	if len(name) != 129 || name[64] != '-' || strings.ToLower(name) != name {
		return false
	}
	_, firstErr := hex.DecodeString(name[:64])
	_, secondErr := hex.DecodeString(name[65:])
	return firstErr == nil && secondErr == nil
}

func (t Taker) artifactReferenced(ctx context.Context, root, rel string) (bool, error) {
	conn, err := pgx.Connect(ctx, t.Config.DatabaseURL)
	if err != nil {
		return false, fmt.Errorf("connect workshop: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	used, err := store.New(conn).ArtifactPathReferenced(ctx, store.ArtifactPathReferencedParams{RelativePath: rel, AbsolutePath: filepath.Join(root, rel)})
	if err != nil {
		return false, fmt.Errorf("query artifact reference: %w", err)
	}
	return used, nil
}

func copyArtifact(src, target string) error {
	sum, err := artifact.Verify(src)
	if err != nil {
		return fmt.Errorf("verify source artifact: %w", err)
	}
	parent := filepath.Dir(target)
	if mkdirErr := os.MkdirAll(parent, 0700); mkdirErr != nil {
		return fmt.Errorf("create artifact parent: %w", mkdirErr)
	}
	temp, err := os.MkdirTemp(parent, ".copy-")
	if err != nil {
		return fmt.Errorf("create artifact copy: %w", err)
	}
	defer func() { _ = os.RemoveAll(temp) }()
	err = filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		dst := filepath.Join(temp, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("invalid artifact file %q", rel)
		}
		in, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		out, createErr := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if createErr != nil {
			_ = in.Close()
			return createErr
		}
		_, copyErr := io.Copy(out, in)
		outErr := out.Close()
		inErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if outErr != nil {
			return outErr
		}
		return inErr
	})
	if err != nil {
		return fmt.Errorf("copy artifact: %w", err)
	}
	copied, err := artifact.Verify(temp)
	if err != nil {
		return fmt.Errorf("verify copied artifact: %w", err)
	}
	if copied != sum {
		return errors.New("copied artifact differs from source")
	}
	if err := os.Rename(temp, target); err != nil {
		return fmt.Errorf("publish artifact copy: %w", err)
	}
	return nil
}
func prune(root string, keep int) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read checkpoints: %w", err)
	}
	type item struct {
		name string
		at   time.Time
	}
	var items []item
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(root, entry.Name(), "checkpoint.json"))
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return fmt.Errorf("read checkpoint: %w", readErr)
		}
		var m metadata
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("decode checkpoint: %w", err)
		}
		items = append(items, item{entry.Name(), m.Time})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].at.Equal(items[j].at) {
			return items[i].name > items[j].name
		}
		return items[i].at.After(items[j].at)
	})
	for _, item := range items[min(keep, len(items)):] {
		if err := os.RemoveAll(filepath.Join(root, item.name)); err != nil {
			return fmt.Errorf("remove old checkpoint: %w", err)
		}
	}
	return nil
}
