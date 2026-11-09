package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"github.com/dannyota/traquydinh/apps/corpus/internal/vbpl"
)

type redirectTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = r.target.Scheme, r.target.Host
	u.Path = strings.TrimPrefix(u.Path, "/api/qtdc/public")
	clone.URL = &u
	return r.base.RoundTrip(clone)
}

type fixtureServer struct {
	files     []map[string]any
	bodies    map[string]string
	missing   bool
	oversized bool
}

func setup(t *testing.T, f *fixtureServer) (*Stage, stage.Env) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.oversized {
			_, _ = w.Write(make([]byte, 20_000_001))
			return
		}
		switch {
		case r.URL.Path == "/doc/66518":
			if f.missing {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fixture(t, w, "detail-66518.json")
		case strings.Contains(r.URL.Path, "/tree/"):
			fixture(t, w, "tree-66518.json")
		case strings.HasSuffix(r.URL.Path, "/diagram"):
			fixture(t, w, "diagram-66518.json")
		case strings.HasSuffix(r.URL.Path, "/files"):
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": f.files})
		case strings.HasPrefix(r.URL.Path, "/attachment/"):
			body, ok := f.bodies[strings.TrimPrefix(r.URL.Path, "/attachment/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	http.DefaultTransport = redirectTransport{target: target, base: old}
	t.Cleanup(func() { http.DefaultTransport = old })
	return New(vbpl.New("test")), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
}
func fixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", name))
	if err != nil {
		t.Error(err)
		return
	}
	_, _ = w.Write(b)
}
func entry(name, path string) map[string]any {
	return map[string]any{"fileName": name, "fullPath": path, "presignedUrl": "https://example.test/attachment/" + path}
}
func process(t *testing.T, s *Stage, env stage.Env) stage.Output {
	t.Helper()
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestFetch_SameContentSameHash(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, e := setup(t, f)
	a, b := process(t, s, e), process(t, s, e)
	if a.Artifact.SHA256 != b.Artifact.SHA256 {
		t.Fatal("hash changed")
	}
}
func TestFetch_ChangedFileChangesHash(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, e := setup(t, f)
	a := process(t, s, e)
	f.bodies["a"] = "two"
	b := process(t, s, e)
	if a.Artifact.SHA256 == b.Artifact.SHA256 {
		t.Fatal("hash unchanged")
	}
}
func TestFetch_RetryAfterRenameWithChangedContent(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, e := setup(t, f)
	a := process(t, s, e)
	f.bodies["a"] = "two"
	b := process(t, s, e)
	if a.Artifact.Path == b.Artifact.Path {
		t.Fatal("same path")
	}
	for _, p := range []string{a.Artifact.Path, b.Artifact.Path} {
		if _, err := artifact.Verify(p); err != nil {
			t.Fatal(err)
		}
	}
}
func TestFetch_MissingAttachmentIsGap(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{}}
	s, e := setup(t, f)
	out := process(t, s, e)
	res, err := s.Resolve(context.Background(), e, stage.Run{}, stage.Item{}, *out.Artifact)
	if err != nil || len(res.Gaps) != 1 || res.Gaps[0] != (stage.Gap{Kind: "missing_attachment", Detail: "a"}) {
		t.Fatalf("%+v %v", res, err)
	}
}
func TestFetch_FileOverLimitIsGap(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "long"}}
	s, e := setup(t, f)
	s.WithFileLimit(2)
	out := process(t, s, e)
	res, err := s.Resolve(context.Background(), e, stage.Run{}, stage.Item{}, *out.Artifact)
	if err != nil || len(res.Gaps) != 1 || res.Gaps[0].Kind != "file_too_large" {
		t.Fatalf("%+v %v", res, err)
	}
}
func TestFetch_DetailNotFoundIsGap(t *testing.T) {
	s, e := setup(t, &fixtureServer{missing: true})
	out := process(t, s, e)
	if out.Artifact != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0] != (stage.Gap{Kind: "not_found", Detail: "66518"}) {
		t.Fatalf("%+v", out)
	}
}
func TestFetch_ResponseOverLimitIsGap(t *testing.T) {
	s, e := setup(t, &fixtureServer{oversized: true})
	out := process(t, s, e)
	if out.Artifact != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "response_too_large" {
		t.Fatalf("%+v", out)
	}
}
func TestFetch_HostileFileNames(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("../../x", "a"), entry("same", "b"), entry("same", "c"), entry("é", "d"), entry("e\u0301", "e")}, bodies: map[string]string{"a": "a", "b": "b", "c": "c", "d": "d", "e": "e"}}
	s, e := setup(t, f)
	out := process(t, s, e)
	b, err := os.ReadFile(filepath.Join(out.Artifact.Path, "files.json"))
	if err != nil {
		t.Fatal(err)
	}
	var files []file
	if err = json.Unmarshal(b, &files); err != nil || len(files) != 5 {
		t.Fatalf("%s %v", b, err)
	}
	for _, item := range files {
		if _, err = os.Stat(filepath.Join(out.Artifact.Path, "files", item.SHA256)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = artifact.Verify(out.Artifact.Path); err != nil {
		t.Fatal(err)
	}
}
