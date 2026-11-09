//go:build integration && faultinject

package proof

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type gateway struct {
	server          *httptest.Server
	details         atomic.Int64
	blockAfterFirst chan struct{}
}

func newGateway(t *testing.T) *gateway {
	t.Helper()
	tree, err := os.ReadFile(filepath.Join("..", "..", "internal", "vbpl", "testdata", "tree-67878.json"))
	if err != nil {
		t.Fatal(err)
	}
	g := &gateway{}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case path == "/doc/all":
			items := make([]map[string]string, 0, 12)
			for i := 1; i <= 12; i++ {
				items = append(items, map[string]string{"id": fmt.Sprintf("%d", i), "docNum": fmt.Sprintf("%d/2026", i), "title": "Document"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": 12, "items": items}})
		case strings.HasPrefix(path, "/doc/provision/tree/"):
			_, _ = w.Write(tree)
		case strings.HasPrefix(path, "/doc/minio/buckets/vbpl/folders/") && strings.HasSuffix(path, "/files"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/doc/minio/buckets/vbpl/folders/"), "/files")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{{"fileName": "attachment.txt", "fullPath": id + "/attachment.txt", "size": 7, "presignedURL": g.server.URL + "/attachment/" + id}}})
		case strings.HasPrefix(path, "/attachment/"):
			_, _ = w.Write([]byte("content"))
		case strings.HasPrefix(path, "/doc/") && strings.HasSuffix(path, "/diagram"):
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"documentNamesByType": map[string]any{"10": []map[string]string{{"id": "1", "name": "Document 1"}}}}})
		case strings.HasPrefix(path, "/doc/"):
			count := g.details.Add(1)
			if count > 1 && g.blockAfterFirst != nil {
				select {
				case <-g.blockAfterFirst:
				case <-r.Context().Done():
					return
				}
			}
			id := strings.TrimPrefix(path, "/doc/")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": id, "docNum": id + "/2026", "title": "Document " + id, "hasContent": true}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.server.Close)
	return g
}
