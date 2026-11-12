package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/sourcegate"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func conflictingAttachmentFixture(t *testing.T, ids ...string) (*Stage, stage.Env) {
	t.Helper()
	id := "66518"
	if len(ids) > 0 {
		id = ids[0]
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			e := map[string]any{"fileName": "a", "fullPath": "a", "size": 3, "presignedUrl": "http://" + r.Host + "/attachment/a"}
			good := map[string]any{"fileName": "good", "fullPath": "good", "size": 3, "presignedUrl": "http://" + r.Host + "/attachment/good"}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{e, e, good}})
		case r.URL.Path == "/attachment/good":
			_, _ = w.Write([]byte("one"))
		case strings.HasPrefix(r.URL.Path, "/attachment/"):
			body := "one"
			if calls.Add(1)%2 == 0 {
				body = "two"
			}
			_, _ = w.Write([]byte(body))
		case r.URL.Path == "/doc/"+id:
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": id, "title": "fixture"}})
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return New(testClient(srv.URL)), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
}
func TestFetch_ConflictingAttachmentEvidenceIsMissing(t *testing.T) {
	s, env := conflictingAttachmentFixture(t)
	ctx, run, item := context.Background(), stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"}
	out, err := s.Process(ctx, env, run, item)
	if err != nil || out.State != "done" || out.Artifact == nil || len(out.Gaps) != 0 || parseReadMessage(out.Message).counts["failure_conflict"] != 1 {
		t.Fatalf("output=%+v error=%v", out, err)
	}
	dir := env.Artifacts.Abs(out.Artifact.Path)
	data, err := os.ReadFile(filepath.Join(dir, "files.json"))
	if err != nil {
		t.Fatal(err)
	}
	var files []file
	if err = json.Unmarshal(data, &files); err != nil || len(files) != 1 || files[0].Name != "good" {
		t.Fatalf("files=%+v error=%v", files, err)
	}
	blobs, err := os.ReadDir(filepath.Join(dir, "files"))
	if err != nil || len(blobs) != 1 || blobs[0].Name() != files[0].SHA256 {
		t.Fatalf("retained blobs=%v error=%v", blobs, err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil || string(data) != `{"missing":["a"],"too_large":[]}` {
		t.Fatalf("report=%s error=%v", data, err)
	}
	for _, o := range out.Observations {
		if o.Name != nil && *o.Name == "a" {
			t.Fatalf("conflicting observation=%+v", o)
		}
	}
	for range 2 {
		if err = s.VerifyAcquisition(ctx, env, run, item, *out.Artifact); err != nil {
			t.Fatal(err)
		}
		resolution, e := s.Resolve(ctx, env, run, item, *out.Artifact)
		if e != nil || !reflect.DeepEqual(resolution.Gaps, []stage.Gap{{Kind: "missing_attachment", Detail: "a"}}) {
			t.Fatalf("resolution=%+v error=%v", resolution, e)
		}
	}
}

func TestFetch_ConflictDoesNotHideSourcePause(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 3 {
			w.WriteHeader(403)
			return
		}
		body := "one"
		if n == 2 {
			body = "two"
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s := New(testClient(srv.URL))
	entry := vbpl.File{Name: "a", URL: srv.URL, Size: 3}
	err := s.writeBundle(context.Background(), t.TempDir(), "1", json.RawMessage(`{}`), json.RawMessage(`{}`), json.RawMessage(`{}`), []vbpl.File{entry, entry, entry})
	if !errors.Is(err, sourcegate.ErrSourcePaused) {
		t.Fatalf("conflict hid pause: %v", err)
	}
}
