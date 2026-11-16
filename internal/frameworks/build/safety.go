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

func realPath(path string) (string, error) {
	current := string(filepath.Separator)
	if !filepath.IsAbs(path) {
		var err error
		current, err = os.Getwd()
		if err != nil {
			return "", hide("resolve path", err)
		}
		current, err = filepath.EvalSymlinks(current)
		if err != nil {
			return "", hide("resolve path", err)
		}
	}
	missing := false
	directory := true
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part != "" && !directory {
			return "", fmt.Errorf("resolve path: parent is not a directory")
		}
		switch part {
		case "", ".":
			continue
		case "..":
			if missing {
				return "", fmt.Errorf("resolve path: missing parent")
			}
			current = filepath.Dir(current)
			continue
		}
		current = filepath.Join(current, part)
		if missing {
			continue
		}
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			missing = true
			continue
		}
		if err != nil {
			return "", hide("resolve path", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			current, err = filepath.EvalSymlinks(current)
			if err != nil {
				return "", hide("resolve path", err)
			}
			info, err = os.Stat(current)
			if err != nil {
				return "", hide("resolve path", err)
			}
		}
		directory = info.IsDir()
	}
	return current, nil
}

func checkedDir(path string) (string, error) {
	path, err := realPath(path)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", hide("resolve home", err)
	}
	if err := checkSync(path, home); err != nil {
		return "", err
	}
	return path, nil
}

func inside(path, root string) bool {
	rel, err := filepath.Rel(strings.ToLower(root), strings.ToLower(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func contained(path, root string) bool {
	rel, err := filepath.Rel(root, path)
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

func prepare() (string, string, error) {
	var source, work string
	for _, name := range []string{"FRAMEWORKS_SOURCE_DIR", "FRAMEWORKS_WORK_DIR"} {
		value := os.Getenv(name)
		if value == "" {
			return "", "", fmt.Errorf("%s is required", name)
		}
		path, err := checkedDir(value)
		if err != nil {
			return "", "", err
		}
		if name == "FRAMEWORKS_WORK_DIR" {
			work = path
		} else {
			source = path
		}
	}
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return "", "", hide("disable core dumps", err)
	}
	tmp, err := checkedDir(filepath.Join(work, "tmp"))
	if err != nil {
		return "", "", err
	}
	if !contained(tmp, work) {
		return "", "", fmt.Errorf("temporary directory outside work directory")
	}
	if err := makeDirectory(tmp); err != nil {
		return "", "", err
	}
	if err := os.Setenv("TMPDIR", tmp); err != nil {
		return "", "", hide("set temporary directory", err)
	}
	return source, work, nil
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

func removeTemp(path string) error {
	resolved, err := checkedDir(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return hide("inspect temporary step", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("temporary step is a symlink")
	}
	err = filepath.WalkDir(resolved, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return hide("inspect temporary step", walkErr)
		}
		if entry.IsDir() {
			_, checkErr := checkedDir(path)
			return checkErr
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.RemoveAll(resolved); err != nil {
		return hide("remove temporary step", err)
	}
	return nil
}
