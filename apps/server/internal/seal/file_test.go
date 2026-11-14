package seal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile_Validation(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		mode          os.FileMode
		valid         bool
	}{
		{"line", key(1), 0600, true}, {"newline", key(1) + "\n", 0600, true}, {"rotation", key(1) + "\n" + key(2) + "\n", 0400, true},
		{"group", key(1), 0640, false}, {"world", key(1), 0604, false}, {"group execute", key(1), 0610, false}, {"malformed", "synthetic invalid key", 0600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "account_keys")
			if err := os.WriteFile(path, []byte(tc.content), tc.mode); err != nil {
				t.Fatal(err)
			}
			_, err := LoadFile(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err != nil && (!strings.Contains(err.Error(), "SERVER_ACCOUNT_KEYS_FILE") || strings.Contains(err.Error(), tc.content)) {
				t.Fatal("unsafe error")
			}
		})
	}
	for _, path := range []string{"", filepath.Join(t.TempDir(), "absent")} {
		if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "SERVER_ACCOUNT_KEYS_FILE") {
			t.Fatal("missing file accepted")
		}
	}
}

func TestLoadFile_SymlinkValidatesOpenedTarget(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target"), filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte(key(1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(link); err != nil {
		t.Fatal("private regular target refused")
	}
	if err := os.Chmod(target, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(link); err == nil {
		t.Fatal("unsafe symlink target accepted")
	}
}
