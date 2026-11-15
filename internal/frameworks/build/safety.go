package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type hiddenError struct{ cause error }

func (e *hiddenError) Error() string { return "operation failed" }
func (e *hiddenError) Unwrap() error { return e.cause }

func hide(message string, err error) error {
	return fmt.Errorf("%s: %w", message, &hiddenError{cause: err})
}

func realPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", hide("resolve path", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return real, nil
	}
	if !errors.Is(err, os.ErrNotExist) || filepath.Dir(abs) == abs {
		return "", hide("resolve path", err)
	}
	parent, err := realPath(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func inside(path, root string) bool {
	rel, err := filepath.Rel(strings.ToLower(root), strings.ToLower(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func checkSync(path, home string) error {
	home, err := realPath(home)
	if err != nil {
		return err
	}
	roots := []string{"Library/Mobile Documents", "Library/CloudStorage", "Dropbox", "Google Drive", "Desktop", "Documents"}
	entries, err := os.ReadDir(home)
	if err != nil {
		return hide("inspect sync folders", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(strings.ToLower(entry.Name()), "onedrive") {
			roots = append(roots, entry.Name())
		}
	}
	roots = append(roots, "OneDrive")
	if inside(path, home) && len(path) > len(home) {
		relative := strings.TrimLeft(path[len(home):], string(filepath.Separator))
		first, _, _ := strings.Cut(relative, string(filepath.Separator))
		if strings.HasPrefix(strings.ToLower(first), "onedrive") {
			roots = append(roots, first)
		}
	}
	for _, root := range roots {
		resolved, resolveErr := realPath(filepath.Join(home, root))
		if resolveErr != nil {
			return resolveErr
		}
		if inside(path, resolved) {
			return fmt.Errorf("sync folder refused: %s", resolved)
		}
	}
	data, err := os.ReadFile(filepath.Join(home, ".dropbox/info.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return hide("read sync configuration", err)
	}
	if err == nil {
		var clients map[string]struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(data, &clients); err != nil {
			return hide("decode sync configuration", err)
		}
		for _, client := range clients {
			if client.Path == "" {
				continue
			}
			root, err := realPath(client.Path)
			if err != nil {
				return err
			}
			if inside(path, root) {
				return fmt.Errorf("sync folder refused: %s", root)
			}
		}
	}
	for ancestor := path; ; ancestor = filepath.Dir(ancestor) {
		if info, err := os.Lstat(filepath.Join(ancestor, ".dropbox")); err == nil {
			if !info.IsDir() || ancestor != home {
				return fmt.Errorf("sync folder refused: %s", ancestor)
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return hide("inspect sync marker", err)
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	return nil
}

func prepare() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", hide("resolve home", err)
	}
	var work string
	for _, name := range []string{"FRAMEWORKS_SOURCE_DIR", "FRAMEWORKS_WORK_DIR"} {
		value := os.Getenv(name)
		if value == "" {
			return "", fmt.Errorf("%s is required", name)
		}
		path, err := realPath(value)
		if err != nil {
			return "", err
		}
		if err := checkSync(path, home); err != nil {
			return "", err
		}
		if name == "FRAMEWORKS_WORK_DIR" {
			work = path
		}
	}
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return "", hide("disable core dumps", err)
	}
	tmp := filepath.Join(work, "tmp")
	if err := makeDirectory(tmp); err != nil {
		return "", err
	}
	if err := os.Setenv("TMPDIR", tmp); err != nil {
		return "", hide("set temporary directory", err)
	}
	return work, nil
}

func makeDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return hide("create directory", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return hide("inspect directory", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("directory refused: %s", path)
	}
	return nil
}
