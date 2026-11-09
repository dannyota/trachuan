// Package artifact publishes and verifies immutable artifact directories.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Store holds artifacts under Root.
type Store struct {
	Root string
	sync func(string) error
}

// Abs resolves an artifact path against Root, preserving absolute paths.
func (s Store) Abs(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(s.Root, rel)
}

// Result identifies a published artifact and its input key.
type Result struct {
	Path, SHA256, InputHash string
	Adopted                 bool
}

func validKey(stage, input, recipe string) error {
	if stage == "" || strings.Trim(stage, "abcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return fmt.Errorf("invalid stage %q", stage)
	}
	for _, h := range []string{input, recipe} {
		if len(h) != 64 || strings.ToLower(h) != h {
			return fmt.Errorf("invalid hash %q", h)
		}
		if _, err := hex.DecodeString(h); err != nil {
			return fmt.Errorf("invalid hash: %w", err)
		}
	}
	return nil
}

// Open verifies an existing artifact or reports that its directory is absent.
func (s Store) Open(stage, inputHash, recipeHash string) (Result, bool, error) {
	if err := validKey(stage, inputHash, recipeHash); err != nil {
		return Result{}, false, err
	}
	rel := filepath.Join(stage, inputHash+"-"+recipeHash)
	path := s.Abs(rel)
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("inspect artifact: %w", err)
	}
	sum, err := Verify(path)
	if err != nil {
		return Result{}, false, fmt.Errorf("verify artifact: %w", err)
	}
	return Result{Path: rel, SHA256: sum, InputHash: inputHash}, true, nil
}

// Publish writes an artifact using the supplied input hash.
func (s Store) Publish(ctx context.Context, stage, inputHash, recipeHash string, write func(string) error) (Result, error) {
	if err := validKey(stage, inputHash, recipeHash); err != nil {
		return Result{}, err
	}
	return s.publish(ctx, stage, inputHash, recipeHash, write, false)
}

// PublishContent uses the manifest hash as the artifact's input hash.
func (s Store) PublishContent(ctx context.Context, stage, recipeHash string, write func(string) error) (Result, error) {
	if err := validKey(stage, strings.Repeat("0", 64), recipeHash); err != nil {
		return Result{}, err
	}
	return s.publish(ctx, stage, "", recipeHash, write, true)
}

func (s Store) publish(ctx context.Context, stage, inputHash, recipeHash string, write func(string) error, content bool) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("publish artifact: %w", err)
	}
	for _, dir := range []string{filepath.Join(s.Root, ".tmp"), filepath.Join(s.Root, ".locks"), filepath.Join(s.Root, stage)} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return Result{}, fmt.Errorf("create artifact directory: %w", err)
		}
	}
	if err := syncPath(s.Root, s.sync); err != nil {
		return Result{}, fmt.Errorf("sync artifact root: %w", err)
	}
	temp, err := os.MkdirTemp(filepath.Join(s.Root, ".tmp"), "artifact-")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary artifact: %w", err)
	}
	defer func() { _ = os.RemoveAll(temp) }()
	if err = write(temp); err != nil {
		return Result{}, fmt.Errorf("write artifact: %w", err)
	}
	if _, err = os.Lstat(filepath.Join(temp, manifestName)); err == nil {
		return Result{}, fmt.Errorf("writer created reserved manifest")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("inspect manifest: %w", err)
	}
	data, err := manifest(temp)
	if err != nil {
		return Result{}, err
	}
	if err = os.WriteFile(filepath.Join(temp, manifestName), data, 0600); err != nil {
		return Result{}, fmt.Errorf("write manifest: %w", err)
	}
	if err = syncTree(temp, s.sync); err != nil {
		return Result{}, fmt.Errorf("sync artifact: %w", err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	if content {
		inputHash = sum
	}
	key := stage + "-" + inputHash + "-" + recipeHash
	f, err := lock(ctx, filepath.Join(s.Root, ".locks", key+".lock"))
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = f.Close() }()
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	rel := filepath.Join(stage, inputHash+"-"+recipeHash)
	path := s.Abs(rel)
	_, err = os.Lstat(path)
	if err == nil {
		existing, verifyErr := Verify(path)
		if verifyErr == nil && (!content || existing == inputHash) {
			if syncErr := syncPath(filepath.Dir(path), s.sync); syncErr != nil {
				return Result{}, fmt.Errorf("sync artifact parent: %w", syncErr)
			}
			return Result{Path: rel, SHA256: existing, InputHash: inputHash, Adopted: true}, nil
		}
		trash := filepath.Join(s.Root, ".trash")
		if err = os.MkdirAll(trash, 0700); err != nil {
			return Result{}, fmt.Errorf("create trash: %w", err)
		}
		var dest string
		dest, err = os.MkdirTemp(trash, "artifact-")
		if err != nil {
			return Result{}, fmt.Errorf("reserve trash path: %w", err)
		}
		if err = os.Remove(dest); err != nil {
			return Result{}, fmt.Errorf("prepare trash path: %w", err)
		}
		if err = os.Rename(path, dest); err != nil {
			return Result{}, fmt.Errorf("move partial artifact: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, fmt.Errorf("inspect artifact: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("publish artifact: %w", err)
	}
	if err = os.Rename(temp, path); err != nil {
		return Result{}, fmt.Errorf("publish artifact: %w", err)
	}
	if err = syncPath(filepath.Dir(path), s.sync); err != nil {
		return Result{}, fmt.Errorf("sync artifact parent: %w", err)
	}
	return Result{Path: rel, SHA256: sum, InputHash: inputHash}, nil
}
