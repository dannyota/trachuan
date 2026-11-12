package ocrvision

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func userCredentials(endpoint string) []byte {
	return []byte(fmt.Sprintf(`{"type":"authorized_user","client_id":"test","client_secret":"test","refresh_token":"test","token_uri":%q}`, endpoint))
}
func TestAuth_RequiresExplicitFile(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	_, err := NewAuthenticated(context.Background())
	if err == nil || !strings.Contains(err.Error(), "GOOGLE_APPLICATION_CREDENTIALS") {
		t.Fatalf("err=%v", err)
	}
}

func TestAuth_RejectsUnreadableAndNonregularFiles(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	blocked := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(blocked, []byte("test"), 0000); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{blocked, blocked + "missing", filepath.Dir(blocked)} {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
		_, err := NewAuthenticated(context.Background())
		if err == nil || strings.Contains(err.Error(), path) {
			t.Fatalf("path accepted or disclosed: %v", err)
		}
	}
}

func TestAuth_RejectsSymlinkCredentials(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer endpoint.Close()
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.json"), filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(target, userCredentials(endpoint.URL), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", link)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	if _, err := NewAuthenticated(context.Background()); err == nil {
		t.Fatal("accepted credentials from a symlink")
	}
}
