package loadrun

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun_SecretsExcluded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/me":
			_, _ = fmt.Fprint(w, `{"csrf_token":"synthetic-csrf-value"}`)
		case "/api/v1/public/documents":
			_, _ = fmt.Fprint(w, `{"items":[{"key":"vn:fixture:1"}]}`)
		default:
			_, _ = fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	sessions := make([]string, 3)
	for i := range sessions {
		sessions[i] = filepath.Join(dir, fmt.Sprint(i))
		if err := os.WriteFile(sessions[i], []byte("synthetic-session-value"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "out")
	err := Run(context.Background(), Config{Pages: server.URL, API: server.URL, PublicURL: server.URL, Queries: []string{"unique query"}, Sessions: sessions, Phases: []string{"A"}, DurationA: 10 * time.Millisecond, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatal("missing outputs")
	}
	for _, entry := range entries {
		b, err := os.ReadFile(filepath.Join(out, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"synthetic-csrf-value", "synthetic-session-value"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("secret in %s", entry.Name())
			}
		}
	}
}
