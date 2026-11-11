package oauth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreregistered_FileLoad(t *testing.T) {
	if clients, err := LoadPreregistered(""); err != nil || len(clients) != 0 {
		t.Fatal("optional file failed")
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`[{"client_id":"native","client_name":"Native","redirect_uris":["http://localhost/cb"]}]`, true},
		{`[{"client_id":"urn:client:native","client_name":"Native","redirect_uris":["http://localhost/cb"]}]`, true},
		{`[{"client_id":"native","client_name":"Native","redirect_uris":["http://localhost/cb"]},{"client_id":"native","redirect_uris":["http://localhost/cb"]}]`, false},
		{`[{"client_id":"https://example.com/client","redirect_uris":["http://localhost/cb"]}]`, false},
		{`[{"client_id":"native","redirect_uris":["http://external.example/cb"]}]`, false},
		{`[{"client_id":"native","client_secret":"refused","redirect_uris":["http://localhost/cb"]}]`, false},
		{`null`, false}, {`[] {}`, false},
	} {
		path := filepath.Join(t.TempDir(), "clients.json")
		if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		clients, err := LoadPreregistered(path)
		if (err == nil) != tc.valid {
			t.Fatalf("load valid=%v error=%v", tc.valid, err)
		}
		if tc.valid && (len(clients) != 1 || (clients[0].ID != "native" && clients[0].ID != "urn:client:native") || clients[0].Kind != "preregistered" || clients[0].Name != "Native") {
			t.Fatalf("clients %v", clients)
		}
	}
	_, err := LoadPreregistered(filepath.Join(t.TempDir(), "missing"))
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("missing file accepted")
	}
}
