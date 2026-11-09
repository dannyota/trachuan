//go:build integration && faultinject

package proof

import (
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (h *harness) restoreFetch() {
	h.t.Helper()
	h.db.Close()
	parsed, err := url.Parse(h.dbURL)
	if err != nil {
		h.t.Fatal("parse database URL")
	}
	name := parsed.Path[1:]
	parsed.Path = "/postgres"
	admin, err := pgx.Connect(h.ctx, parsed.String())
	if err != nil {
		h.t.Fatal("connect to database administrator")
	}
	defer func() { _ = admin.Close(h.ctx) }()
	identifier := pgx.Identifier{name}.Sanitize()
	if _, dropErr := admin.Exec(h.ctx, "DROP DATABASE "+identifier+" WITH (FORCE)"); dropErr != nil {
		h.t.Fatalf("drop database: %v", dropErr)
	}
	if _, createErr := admin.Exec(h.ctx, "CREATE DATABASE "+identifier); createErr != nil {
		h.t.Fatalf("create database: %v", createErr)
	}
	dumpTool, err := exec.LookPath(h.dump)
	if err != nil {
		h.t.Fatal("locate pg_dump")
	}
	restore := filepath.Join(filepath.Dir(dumpTool), "pg_restore")
	dump := filepath.Join(h.checkpoints, "vn-"+release, "1-fetch", "workshop.dump")
	cmd := exec.Command(restore, "--exit-on-error", "--dbname", h.dbURL, dump)
	if restoreErr := cmd.Run(); restoreErr != nil {
		h.t.Fatalf("restore checkpoint: %v", restoreErr)
	}
	if removeErr := os.RemoveAll(h.artifacts); removeErr != nil {
		h.t.Fatal(removeErr)
	}
	if copyErr := copyTree(filepath.Join(h.checkpoints, "artifacts"), h.artifacts); copyErr != nil {
		h.t.Fatal(copyErr)
	}
	h.db, err = pgxpool.New(h.ctx, h.dbURL)
	if err != nil {
		h.t.Fatal("reopen restored database")
	}
}

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk artifacts: %w", walkErr)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative artifact path: %w", err)
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		input, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open artifact: %w", err)
		}
		defer func() { _ = input.Close() }()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create artifact: %w", err)
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return fmt.Errorf("copy artifact: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close artifact: %w", closeErr)
		}
		return nil
	})
}

func (h *harness) fetchItems() string {
	h.t.Helper()
	var result string
	err := h.db.QueryRow(h.ctx, `SELECT coalesce(string_agg(concat_ws('|',item_key,state,generation,attempt,artifact_id,input_hash,recipe_hash,updated_at), E'\n' ORDER BY item_key), '') FROM run_item WHERE stage='fetch'`).Scan(&result)
	if err != nil {
		h.t.Fatal(err)
	}
	return result
}
