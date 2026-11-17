package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrameworksReleaseRow_WritesPrivateMetadataWithoutWorkshop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	work := t.TempDir()
	t.Setenv("FRAMEWORKS_WORK_DIR", work)
	t.Setenv("CORPUS_DATABASE_URL", "invalid workshop must stay unread")
	var stderr bytes.Buffer
	load := func(_ context.Context, rel string) ([]byte, error) {
		if rel != "20990101_1" {
			t.Fatal("wrong release requested")
		}
		return []byte(`{"kind":"frameworks","rel":"20990101_1","schema_version":1,"extensions":{"vector":"invented"}}`), nil
	}
	if code := runFrameworksReleaseRowWith(t.Context(), "20990101_1", &stderr, load); code != 0 {
		t.Fatalf("code=%d error=%s", code, stderr.String())
	}
	path := filepath.Join(work, "20990101_1", "release", "release.json")
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte(`"extensions"`)) {
		t.Fatal("missing release row")
	}
	for _, tc := range []struct {
		path string
		mode os.FileMode
	}{{filepath.Dir(path), 0700}, {path, 0600}} {
		info, err := os.Stat(tc.path)
		if err != nil || info.Mode().Perm() != tc.mode {
			t.Fatalf("private output mode for %s", filepath.Base(tc.path))
		}
	}
	if code := runFrameworksReleaseRowWith(t.Context(), "20990101_1", &stderr, load); code == 0 {
		t.Fatal("overwrote release row")
	}
}

func TestFrameworksReleaseRow_RejectsTraversalAndSymlinkOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	work := t.TempDir()
	t.Setenv("FRAMEWORKS_WORK_DIR", work)
	load := func(context.Context, string) ([]byte, error) { return []byte(`{}`), nil }
	for _, rel := range []string{"../20990101", "20990101/../../escape", "", "20990101_bad"} {
		var stderr bytes.Buffer
		if code := runFrameworksReleaseRowWith(t.Context(), rel, &stderr, load); code == 0 {
			t.Fatalf("accepted rel %q", rel)
		}
	}
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "20990101"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(work, "20990101/release")); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := runFrameworksReleaseRowWith(t.Context(), "20990101", &stderr, load); code == 0 {
		t.Fatal("accepted symlink release directory")
	}
	if _, err := os.Stat(filepath.Join(outside, "release.json")); !os.IsNotExist(err) {
		t.Fatal("wrote outside work directory")
	}
}

func TestFrameworksReleaseRow_RefusesSyncedWorkBeforeLoadingOrWriting(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "symlink"}[linked], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			synced := filepath.Join(home, "Dropbox", "work")
			if err := os.MkdirAll(synced, 0700); err != nil {
				t.Fatal(err)
			}
			work := synced
			if linked {
				work = filepath.Join(home, "work-link")
				if err := os.Symlink(synced, work); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("FRAMEWORKS_WORK_DIR", work)
			loaded := false
			load := func(context.Context, string) ([]byte, error) { loaded = true; return []byte(`{}`), nil }
			var stderr bytes.Buffer
			code := runFrameworksReleaseRowWith(t.Context(), "20990101", &stderr, load)
			if code != 1 || !strings.Contains(stderr.String(), "sync folder refused") {
				t.Fatal("accepted synced work directory")
			}
			if loaded {
				t.Fatal("read metadata before refusing synced work directory")
			}
			if _, err := os.Stat(filepath.Join(synced, "20990101")); !os.IsNotExist(err) {
				t.Fatal("created output in synced work directory")
			}
		})
	}
}

func TestFrameworksReleaseRow_RejectsUnsupportedSuffixBeforeLoading(t *testing.T) {
	for _, rel := range []string{"20990101_", "20990101_100", "20990101_1234"} {
		var stderr bytes.Buffer
		loaded := false
		load := func(context.Context, string) ([]byte, error) { loaded = true; return []byte(`{}`), nil }
		code := runFrameworksReleaseRowWith(t.Context(), rel, &stderr, load)
		if code != 2 || loaded {
			t.Fatalf("accepted unsupported suffix %s", rel)
		}
		if _, err := frameworksReleaseRowConfig("postgres://builder@localhost/postgres", rel); err == nil {
			t.Fatalf("configured unsupported suffix %s", rel)
		}
	}
	for _, rel := range []string{"20990101", "20990101_1", "20990101_99"} {
		if !validFrameworksRelease(rel) {
			t.Fatalf("refused supported suffix %s", rel)
		}
	}
}

func TestFrameworksReleaseRow_ConfigIsLocalAndReadOnly(t *testing.T) {
	for _, raw := range []string{"", "postgres://builder@example.invalid/postgres", "invalid"} {
		if _, err := frameworksReleaseRowConfig(raw, "20990101"); err == nil {
			t.Fatal("accepted unsafe builder URL")
		}
	}
	cfg, err := frameworksReleaseRowConfig("postgres://builder@localhost/postgres?default_transaction_read_only=off", "20990101")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "frameworks_20990101" || cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] != "on" {
		t.Fatal("missing read-only database binding")
	}
}
