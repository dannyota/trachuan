package fetch_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestConflictBundle_BuildIdentityFixture(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			entry := map[string]any{"fileName": "a", "fullPath": "a", "size": 3, "presignedUrl": "http://" + r.Host + "/attachment/a"}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{entry, entry}})
		case r.URL.Path == "/attachment/a":
			body := "one"
			if calls.Add(1) == 2 {
				body = "two"
			}
			_, _ = w.Write([]byte(body))
		case r.URL.Path == "/doc/1":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"1","docNum":"42/2026/QD","title":"fixture"}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		}
	}))
	defer server.Close()
	client := vbpl.NewWithBase(server.URL, "test", vbpl.WithTestHTTP(), vbpl.WithLimits(100000, 16), vbpl.WithBackoffs())
	st := fetch.New(client)
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	out, err := st.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
	if err != nil {
		t.Fatal(err)
	}
	item := build.InventoryItem{Stage: "fetch", ItemKey: "vn:vbpl:1", State: out.State}
	if out.Artifact != nil {
		item.Path = &out.Artifact.Path
		item.ArtifactSHA256 = &out.Artifact.SHA256
	}
	identities, err := build.IdentityInventory(build.Inventory{ExpectedKeys: []string{"vn:vbpl:1"}, Items: []build.InventoryItem{item}}, env.Artifacts)
	if err != nil || identities.Documents["vn:vbpl:1"].Number != "42/2026/QD" {
		t.Fatalf("build identities=%+v error=%v", identities, err)
	}
}
