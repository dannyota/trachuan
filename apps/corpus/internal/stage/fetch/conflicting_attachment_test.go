package fetch

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
)

func conflictingAttachmentFixture(t *testing.T) (*Stage, stage.Env) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			e := map[string]any{"fileName": "a", "fullPath": "a", "size": 3, "presignedUrl": "http://" + r.Host + "/attachment/a"}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{e, e}})
		case strings.HasPrefix(r.URL.Path, "/attachment/"):
			body := "one"
			if calls.Add(1)%2 == 0 {
				body = "two"
			}
			_, _ = w.Write([]byte(body))
		case r.URL.Path == "/doc/66518":
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"66518","title":"fixture"}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return New(testClient(srv.URL)), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
}
func TestFetch_ConflictingAttachmentEvidenceIsDoneWithGap(t *testing.T) {
	s, env := conflictingAttachmentFixture(t)
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil || out.State != "done" || out.Artifact != nil || len(out.Gaps) != 1 || out.Gaps[0].Kind != "invalid_source_data" || out.Gaps[0].Origin != stage.OriginSourceData {
		t.Fatalf("output=%+v error=%v", out, err)
	}
}
