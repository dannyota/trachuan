package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"danny.vn/trachuan/internal/frameworks/build"
	"github.com/jackc/pgx/v5/pgxpool"
)

func validFrameworksRelease(rel string) bool {
	return regexp.MustCompile(`^[0-9]{8}(_[0-9]{1,2})?$`).MatchString(rel)
}

func runFrameworksReleaseRow(ctx context.Context, rel string, stderr io.Writer) int {
	return runFrameworksReleaseRowWith(ctx, rel, stderr, func(ctx context.Context, rel string) ([]byte, error) {
		return readFrameworksReleaseRow(ctx, os.Getenv("CORPUS_BUILDER_DATABASE_URL"), rel)
	})
}

func runFrameworksReleaseRowWith(ctx context.Context, rel string, stderr io.Writer, load func(context.Context, string) ([]byte, error)) int {
	fail := func(code int, err error) int { _, _ = fmt.Fprintln(stderr, err); return code }
	if !validFrameworksRelease(rel) {
		return fail(2, errors.New("invalid frameworks release"))
	}
	work := os.Getenv("FRAMEWORKS_WORK_DIR")
	if work == "" {
		return fail(1, errors.New("FRAMEWORKS_WORK_DIR is required"))
	}
	work, err := build.CheckedDirectory(work)
	if err != nil {
		return fail(1, err)
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		return fail(1, errors.New("open frameworks work directory failed"))
	}
	defer func() { _ = root.Close() }()
	for _, dir := range []string{rel, filepath.Join(rel, "release")} {
		if err = root.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return fail(1, errors.New("create frameworks release directory failed"))
		}
		info, statErr := root.Lstat(dir)
		if statErr != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			return fail(1, errors.New("frameworks release directory must be private"))
		}
	}
	data, err := load(ctx, rel)
	if err != nil {
		return fail(1, err)
	}
	path := filepath.Join(rel, "release", "release.json")
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fail(1, errors.New("create frameworks release output failed"))
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = root.Remove(path)
		return fail(1, errors.New("write frameworks release output failed"))
	}
	return 0
}

func frameworksReleaseRowConfig(builderURL, rel string) (*pgxpool.Config, error) {
	if !validFrameworksRelease(rel) {
		return nil, errors.New("invalid frameworks release")
	}
	if err := localFrameworkDatabase(builderURL); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(builderURL)
	if err != nil {
		return nil, errors.New("invalid frameworks builder URL")
	}
	cfg.ConnConfig.Database = "frameworks_" + rel
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.MaxConns = 1
	return cfg, nil
}

func readFrameworksReleaseRow(ctx context.Context, builderURL, rel string) ([]byte, error) {
	cfg, err := frameworksReleaseRowConfig(builderURL, rel)
	if err != nil {
		return nil, err
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("open frameworks release metadata failed")
	}
	defer db.Close()
	var raw []byte
	if err = db.QueryRow(ctx, `SELECT to_jsonb(r)-'singleton' FROM release r WHERE singleton`).Scan(&raw); err != nil {
		return nil, errors.New("read frameworks release metadata failed")
	}
	row := map[string]json.RawMessage{}
	if err = json.Unmarshal(raw, &row); err != nil {
		return nil, errors.New("decode frameworks release metadata failed")
	}
	var kind, rowRel string
	if json.Unmarshal(row["kind"], &kind) != nil || json.Unmarshal(row["rel"], &rowRel) != nil || kind != "frameworks" || rowRel != rel {
		return nil, errors.New("frameworks release metadata identity mismatch")
	}
	rows, err := db.Query(ctx, `SELECT extname,extversion FROM pg_extension WHERE extname<>'plpgsql' ORDER BY extname`)
	if err != nil {
		return nil, errors.New("read frameworks release extensions failed")
	}
	defer rows.Close()
	extensions := map[string]string{}
	for rows.Next() {
		var name, version string
		if err = rows.Scan(&name, &version); err != nil {
			return nil, errors.New("read frameworks release extension failed")
		}
		extensions[name] = version
	}
	if rows.Err() != nil {
		return nil, errors.New("read frameworks release extensions failed")
	}
	row["extensions"], err = json.Marshal(extensions)
	if err != nil {
		return nil, fmt.Errorf("encode frameworks extensions: %w", err)
	}
	data, err := json.Marshal(row)
	if err != nil {
		return nil, fmt.Errorf("encode frameworks release metadata: %w", err)
	}
	return data, nil
}
