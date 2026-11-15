package build

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestRun_RequiresDirectories(t *testing.T) {
	for _, name := range []string{"FRAMEWORKS_SOURCE_DIR", "FRAMEWORKS_WORK_DIR"} {
		t.Run(name, func(t *testing.T) {
			fixture(t)
			t.Setenv(name, "")
			require.ErrorContains(t, Run(context.Background(), "20990101", &bytes.Buffer{}), name)
		})
	}
}

func TestRun_RefusesSyncFolders(t *testing.T) {
	for _, name := range []string{"FRAMEWORKS_SOURCE_DIR", "FRAMEWORKS_WORK_DIR"} {
		for _, folder := range []string{"Library/Mobile Documents", "Library/CloudStorage/Provider", "Dropbox", "Google Drive", "OneDrive", "OneDrive - Company", "Desktop", "Documents", "reported", "marker", "symlink"} {
			t.Run(name+"/"+folder, func(t *testing.T) {
				fixture(t)
				home := os.Getenv("HOME")
				root := filepath.Join(home, folder)
				require.NoError(t, os.MkdirAll(root, 0700))
				switch folder {
				case "reported":
					require.NoError(t, os.Mkdir(filepath.Join(home, ".dropbox"), 0700))
					data, err := json.Marshal(map[string]any{"business": map[string]string{"path": root}})
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(home, ".dropbox/info.json"), data, 0600))
				case "marker":
					require.NoError(t, os.WriteFile(filepath.Join(root, ".dropbox"), nil, 0600))
				case "symlink":
					root = filepath.Join(home, "Dropbox")
					require.NoError(t, os.Mkdir(root, 0700))
					link := filepath.Join(home, "alias")
					require.NoError(t, os.Symlink(root, link))
					t.Setenv(name, link)
				}
				if folder != "symlink" {
					t.Setenv(name, filepath.Join(root, "nested"))
				}
				err := Run(context.Background(), "20990101", &bytes.Buffer{})
				require.ErrorContains(t, err, "sync folder refused")
				if folder == "Library/CloudStorage/Provider" {
					require.Contains(t, err.Error(), "CloudStorage")
				} else {
					require.Contains(t, err.Error(), filepath.Base(root))
				}
			})
		}
	}
}

func TestRun_ProcessProtection(t *testing.T) {
	work := fixture(t)
	require.NoError(t, Run(context.Background(), "20990101", &bytes.Buffer{}))
	path, err := filepath.EvalSymlinks(os.Getenv("TMPDIR"))
	require.NoError(t, err)
	realWork, err := filepath.EvalSymlinks(work)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(realWork, "tmp"), path)
	var limit unix.Rlimit
	require.NoError(t, unix.Getrlimit(unix.RLIMIT_CORE, &limit))
	require.Zero(t, limit.Cur)
	require.Zero(t, limit.Max)
}

func TestCheckSync_CaseAndClientConfiguration(t *testing.T) {
	for _, folder := range []string{"dropbox", "onedrive - Company"} {
		t.Run(folder, func(t *testing.T) {
			home := t.TempDir()
			home, err := filepath.EvalSymlinks(home)
			require.NoError(t, err)
			root := filepath.Join(home, folder)
			require.NoError(t, os.Mkdir(root, 0700))
			require.ErrorContains(t, checkSync(filepath.Join(root, "nested"), home), "sync folder refused")
		})
	}
	t.Run("configuration is not a root marker", func(t *testing.T) {
		home := t.TempDir()
		home, err := filepath.EvalSymlinks(home)
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(filepath.Join(home, ".dropbox"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(home, ".dropbox/info.json"), []byte(`{"personal":{"path":"/invented/sync"}}`), 0600))
		require.NoError(t, checkSync(filepath.Join(home, "ordinary"), home))
	})
}

func TestRun_RefusesUncreatedOneDriveVariant(t *testing.T) {
	fixture(t)
	t.Setenv("FRAMEWORKS_WORK_DIR", filepath.Join(os.Getenv("HOME"), "OneDrive - NewCompany", "artifacts"))
	err := Run(context.Background(), "20990101", &bytes.Buffer{})
	require.ErrorContains(t, err, "sync folder refused")
	require.ErrorContains(t, err, "OneDrive - NewCompany")
}
