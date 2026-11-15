package build

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/internal/frameworks/registry"
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

func TestRun_ResolvesSymlinkBeforeParent(t *testing.T) {
	for _, name := range []string{"FRAMEWORKS_SOURCE_DIR", "FRAMEWORKS_WORK_DIR"} {
		t.Run(name, func(t *testing.T) {
			fixture(t)
			root := filepath.Dir(os.Getenv("FRAMEWORKS_SOURCE_DIR"))
			child := filepath.Join(os.Getenv("HOME"), "Documents", "child")
			require.NoError(t, os.MkdirAll(child, 0700))
			link := filepath.Join(root, "link")
			require.NoError(t, os.Symlink(child, link))
			t.Setenv(name, link+"/../source")
			err := Run(context.Background(), "20990101", &bytes.Buffer{})
			require.ErrorContains(t, err, "sync folder refused")
			require.ErrorContains(t, err, "Documents")
		})
	}
}

func TestRun_RefusesNestedSyncDirectories(t *testing.T) {
	for _, directory := range []string{"20990101", "tmp", "20990101/snapshot", "20990101/.tmp-parse-leftover"} {
		for _, kind := range []string{"reported", "marker"} {
			t.Run(directory+"/"+kind, func(t *testing.T) {
				work := fixture(t)
				root := filepath.Join(work, directory)
				require.NoError(t, os.MkdirAll(root, 0700))
				if kind == "marker" {
					require.NoError(t, os.WriteFile(filepath.Join(root, ".dropbox"), nil, 0600))
				} else {
					home := os.Getenv("HOME")
					require.NoError(t, os.Mkdir(filepath.Join(home, ".dropbox"), 0700))
					data, err := json.Marshal(map[string]any{"business": map[string]string{"path": root}})
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(home, ".dropbox/info.json"), data, 0600))
				}
				err := Run(context.Background(), "20990101", &bytes.Buffer{})
				require.ErrorContains(t, err, "sync folder refused")
				require.ErrorContains(t, err, filepath.Base(root))
			})
		}
	}
}

func TestSnapshot_UsesCheckedSource(t *testing.T) {
	fixture(t)
	source := os.Getenv("FRAMEWORKS_SOURCE_DIR")
	link := filepath.Join(filepath.Dir(source), "source-link")
	require.NoError(t, os.Symlink(source, link))
	t.Setenv("FRAMEWORKS_SOURCE_DIR", link)
	checkedSource, _, err := prepare()
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(filepath.Join(filepath.Dir(source), "missing"), link))
	t.Setenv("FRAMEWORKS_SOURCE_DIR", link)
	_, err = snapshot(context.Background(), t.TempDir(), checkedSource)
	require.NoError(t, err)
}

func TestRun_RefusesNestedInputAndArtifactDirectories(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		fixture(t)
		source := os.Getenv("FRAMEWORKS_SOURCE_DIR")
		nested := filepath.Join(source, "nested")
		require.NoError(t, os.Mkdir(nested, 0700))
		require.NoError(t, os.Rename(filepath.Join(source, "fw-alpha.json"), filepath.Join(nested, "fw-alpha.json")))
		require.NoError(t, os.WriteFile(filepath.Join(nested, ".dropbox"), nil, 0600))
		editRegistry(t, func(reg *registry.Registry) { reg.Frameworks[0].Files[0].Path = "nested/fw-alpha.json" })
		err := Run(context.Background(), "20990101", &bytes.Buffer{})
		require.ErrorContains(t, err, "sync folder refused")
		require.ErrorContains(t, err, "nested")
	})
	t.Run("artifact", func(t *testing.T) {
		work := fixture(t)
		require.NoError(t, Run(context.Background(), "20990101", &bytes.Buffer{}))
		nested := filepath.Join(work, "20990101/snapshot/files")
		require.NoError(t, os.WriteFile(filepath.Join(nested, ".dropbox"), nil, 0600))
		err := Run(context.Background(), "20990101", &bytes.Buffer{})
		require.ErrorContains(t, err, "sync folder refused")
		require.ErrorContains(t, err, "files")
	})
}

func TestRealPath_RejectsParentAfterFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, nil, 0600))
	_, err := realPath(file + "/../target")
	require.Error(t, err)
}

func TestRealPath_RelativeToSymlinkWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "actual", "child")
	require.NoError(t, os.MkdirAll(child, 0700))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(child, link))
	t.Chdir(link)
	t.Setenv("PWD", link)
	path, err := realPath("../target")
	require.NoError(t, err)
	realRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(realRoot, "actual", "target"), path)
}

func TestContained_SeparatesCaseSensitivePaths(t *testing.T) {
	require.True(t, contained("/synthetic/work/child", "/synthetic/work"))
	require.False(t, contained("/synthetic/WORK/child", "/synthetic/work"))
	require.False(t, contained("/synthetic/work-other/child", "/synthetic/work"))
}
