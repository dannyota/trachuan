package fetch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestReuseDecision_Conditions(t *testing.T) {
	now := time.Date(2099, 1, 31, 0, 0, 0, 0, time.UTC)
	treeTime := now.Add(-time.Hour)
	cases := []struct {
		name, fresh, old, reason string
		missing                  bool
		age                      bool
		unknown                  bool
	}{
		{"equal", `{"data":{"updatedDate":"x"}}`, `{"data":{"updatedDate":"x"}}`, "reused", false, false, false},
		{"fresh null", `{"data":{"updatedDate":null}}`, `{"data":{"updatedDate":"x"}}`, "null_date", false, false, false},
		{"baseline null", `{"data":{"updatedDate":"x"}}`, `{"data":{}}`, "null_date", false, false, false},
		{"changed", `{"data":{"updatedDate":"y"}}`, `{"data":{"updatedDate":"x"}}`, "date_changed", false, false, false},
		{"missing", `{"data":{"updatedDate":"x"}}`, `{"data":{"updatedDate":"x"}}`, "missing_attachment", true, false, false},
		{"old", `{"data":{"updatedDate":"x"}}`, `{"data":{"updatedDate":"x"}}`, "age", false, true, false},
		{"unknown", `{"data":{"updatedDate":"x"}}`, `{"data":{"updatedDate":"x"}}`, "age", false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			at := treeTime
			if tc.age {
				at = now.Add(-31 * 24 * time.Hour)
			}
			b := &baseline{detail: []byte(tc.old), tree: []byte("tree"), report: report{Missing: []string{}}, observations: []store.RunFetchObservation{{Path: "tree.json", ContentSha256: digest([]byte("tree")), FetchedAt: &at}}}
			if tc.missing {
				b.report.Missing = []string{"x"}
			}
			if tc.unknown {
				b.observations[0].FetchedAt = nil
			}
			if err := b.indexOrigins(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := reuseDecision([]byte(tc.fresh), b, now, 30*24*time.Hour); got != tc.reason {
				t.Fatalf("got %q, want %q", got, tc.reason)
			}
		})
	}
	if got := reuseDecision(nil, &baseline{invalid: true}, now, time.Hour); got != "invalid_baseline" {
		t.Fatalf("invalid: %q", got)
	}
	if got := reuseDecision(nil, nil, now, time.Hour); got != "no_baseline" {
		t.Fatalf("got %q", got)
	}
}

func TestFetch_ReusesBundleAndOrigin(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 31, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	dir := env.Artifacts.Abs(first.Artifact.Path)
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	b := &baseline{dir: dir, detail: read("detail.json"), tree: read("tree.json"), filesJSON: read("files.json"), reportJSON: read("report.json")}
	if err := json.Unmarshal(b.filesJSON, &b.files); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b.reportJSON, &b.report); err != nil {
		t.Fatal(err)
	}
	for _, o := range first.Observations {
		b.observations = append(b.observations, store.RunFetchObservation{RunID: 42, ItemKey: "vn:vbpl:66518", Generation: 2, Attempt: 3, ResourceKey: o.ResourceKey, Source: o.Source, Path: o.Path, Url: o.URL, ContentSha256: o.ContentSHA256, Size: o.Size, FetchedAt: o.FetchedAt})
	}
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
	f.requests.Store(0)
	f.downloads.Store(0)
	now = now.Add(time.Hour)
	second := process(t, s, env)
	if got := f.requests.Load(); got != 2 {
		t.Errorf("gateway calls %d, want 2", got)
	}
	if got := f.downloads.Load(); got != 0 {
		t.Errorf("downloads %d, want 0", got)
	}
	if first.Artifact.SHA256 != second.Artifact.SHA256 {
		t.Errorf("manifest changed")
	}
	for _, o := range second.Observations {
		if o.Path == "tree.json" || strings.HasPrefix(o.Path, "files/") {
			if o.Acquisition != "adopted" || o.FetchedAt == nil || !o.FetchedAt.Equal(now.Add(-time.Hour)) || o.OriginRef == nil {
				t.Errorf("bad origin: %+v", o)
				continue
			}
			var ref []any
			if err := json.Unmarshal([]byte(*o.OriginRef), &ref); err != nil || len(ref) != 5 || ref[0] != float64(42) || ref[4] != o.ResourceKey {
				t.Errorf("bad origin ref %q", *o.OriginRef)
			}
		} else if o.Acquisition != "network" {
			t.Errorf("fresh observation: %+v", o)
		}
	}
}

func TestFetch_AgeDownloadsChangedBytes(t *testing.T) {
	e := entry("a", "a")
	e["size"] = 3
	e["updatedAt"] = "2026-04-14 14:41:19"
	f := &fixtureServer{files: []map[string]any{e}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	b := baselineFromOutput(t, env, first)
	s.WithCache(cacheFixture(t, "one"))
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
	now = now.Add(31 * 24 * time.Hour)
	f.bodies["a"] = "two"
	f.requests.Store(0)
	f.downloads.Store(0)
	out := process(t, s, env)
	if f.requests.Load() != 4 || f.downloads.Load() != 1 {
		t.Fatalf("calls %d, downloads %d", f.requests.Load(), f.downloads.Load())
	}
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "files", digest([]byte("two"))))
	if err != nil || string(data) != "two" {
		t.Fatalf("new bytes %q, error %v", data, err)
	}
}

func baselineFromOutput(t *testing.T, env stage.Env, out stage.Output) *baseline {
	t.Helper()
	dir := env.Artifacts.Abs(out.Artifact.Path)
	read := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	b := &baseline{dir: dir, detail: read("detail.json"), tree: read("tree.json"), filesJSON: read("files.json"), reportJSON: read("report.json")}
	if err := json.Unmarshal(b.filesJSON, &b.files); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b.reportJSON, &b.report); err != nil {
		t.Fatal(err)
	}
	for _, o := range out.Observations {
		b.observations = append(b.observations, store.RunFetchObservation{RunID: 42, ItemKey: "vn:vbpl:66518", Generation: 2, Attempt: 3, ResourceKey: o.ResourceKey, Source: o.Source, Path: o.Path, Url: o.URL, ContentSha256: o.ContentSHA256, Size: o.Size, FetchedAt: o.FetchedAt})
	}
	return b
}

func TestFetch_NullDateCopiesOnlyCurrentAttachments(t *testing.T) {
	a, b := entry("a", "a"), entry("b", "b")
	a["size"], b["size"] = 3, 3
	a["updatedAt"], b["updatedAt"] = "2098-12-01 00:00:00", "2098-12-01 00:00:00"
	f := &fixtureServer{files: []map[string]any{a, b}, bodies: map[string]string{"a": "one", "b": "two"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	base := baselineFromOutput(t, env, first)
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		return base, false, nil
	}
	f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null}}`
	b["updatedAt"] = "2099-01-02 12:00:00"
	f.requests.Store(0)
	f.downloads.Store(0)
	out := process(t, s, env)
	if f.requests.Load() != 4 || f.downloads.Load() != 1 {
		t.Fatalf("calls %d, downloads %d", f.requests.Load(), f.downloads.Load())
	}
	got := map[string]string{}
	for _, o := range out.Observations {
		if o.Name != nil {
			got[*o.Name] = o.Acquisition
		}
	}
	if got["a"] != "adopted" || got["b"] != "network" {
		t.Fatalf("acquisitions %v", got)
	}
}

func TestFetch_CorruptedAttachmentFallsBack(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	base := baselineFromOutput(t, env, first)
	if err := os.WriteFile(filepath.Join(base.dir, "files", base.files[0].SHA256), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		return base, false, nil
	}
	f.requests.Store(0)
	f.downloads.Store(0)
	out := process(t, s, env)
	if f.requests.Load() != 4 || f.downloads.Load() != 1 || out.Artifact.SHA256 != first.Artifact.SHA256 {
		t.Fatalf("calls %d downloads %d output %+v", f.requests.Load(), f.downloads.Load(), out)
	}
}

func TestFetch_UsesLaterProducerObservations(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	now = now.Add(time.Hour)
	later := process(t, s, env)
	if first.Artifact.SHA256 != later.Artifact.SHA256 {
		t.Fatal("bundles differ")
	}
	base := baselineFromOutput(t, env, later)
	for i := range base.observations {
		base.observations[i].RunID = 43
	}
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		return base, false, nil
	}
	now = now.Add(time.Hour)
	reused := process(t, s, env)
	for _, o := range reused.Observations {
		if o.Acquisition != "adopted" {
			continue
		}
		var ref []any
		if o.FetchedAt == nil || !o.FetchedAt.Equal(now.Add(-time.Hour)) || o.OriginRef == nil {
			t.Fatalf("wrong origin %+v", o)
		}
		if err := json.Unmarshal([]byte(*o.OriginRef), &ref); err != nil || ref[0] != float64(43) {
			t.Fatalf("wrong ref %q", *o.OriginRef)
		}
	}
}

func TestFetch_BlockingConditionsReadTreeAndFiles(t *testing.T) {
	for _, condition := range []string{"no_baseline", "invalid_baseline", "fresh_null", "baseline_null", "date_changed", "missing_attachment", "age", "unknown_time"} {
		t.Run(condition, func(t *testing.T) {
			f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
			s, env := setup(t, f)
			now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
			env.Now = func() time.Time { return now }
			initial := process(t, s, env)
			base := baselineFromOutput(t, env, initial)
			switch condition {
			case "no_baseline":
				base = nil
			case "fresh_null":
				f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null}}`
			case "baseline_null":
				base.detail = []byte(`{"data":{"updatedDate":null}}`)
			case "date_changed":
				base.detail = []byte(`{"data":{"updatedDate":"different"}}`)
			case "missing_attachment":
				base.report.Missing = []string{"late"}
			case "age":
				now = now.Add(31 * 24 * time.Hour)
			case "unknown_time":
				for i := range base.observations {
					if base.observations[i].Path == "tree.json" {
						base.observations[i].FetchedAt = nil
					}
				}
			}
			s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
				return base, condition == "invalid_baseline", nil
			}
			f.requests.Store(0)
			f.downloads.Store(0)
			var result readDecision
			out, err := s.processItem(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"}, &result)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"no_baseline": "no_baseline", "invalid_baseline": "invalid_baseline", "fresh_null": "null_date", "baseline_null": "null_date", "date_changed": "date_changed", "missing_attachment": "missing_attachment", "age": "age", "unknown_time": "age"}[condition]
			if result.reason != want {
				t.Fatalf("decision %q, want %q", result.reason, want)
			}
			if out.State != "done" || f.requests.Load() != 4 {
				t.Fatalf("state %s calls %d", out.State, f.requests.Load())
			}
		})
	}
}

func TestFetch_NullDateEmptyURLRemainsMissing(t *testing.T) {
	e := entry("a", "a")
	e["size"] = 3
	e["updatedAt"] = "2026-04-14 14:41:19"
	f := &fixtureServer{files: []map[string]any{e}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	base := baselineFromOutput(t, env, first)
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		return base, false, nil
	}
	f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null}}`
	e["presignedUrl"] = ""
	out := process(t, s, env)
	resolution, err := s.Resolve(context.Background(), env, stage.Run{}, stage.Item{Key: "vn:vbpl:66518"}, *out.Artifact)
	if err != nil || len(resolution.Gaps) != 1 || resolution.Gaps[0].Kind != "missing_attachment" {
		t.Fatalf("resolution %+v: %v", resolution, err)
	}
}

func TestReuseDecision_UnknownAliasTimeBlocksReuse(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	hash := digest([]byte("one"))
	b := &baseline{detail: []byte(`{"data":{"updatedDate":"x"}}`), tree: []byte("tree"), files: []file{{Name: "a", SHA256: hash, Size: 3}}, observations: []store.RunFetchObservation{
		{Path: "tree.json", ContentSha256: digest([]byte("tree")), FetchedAt: &now},
		{Path: "files/" + hash, ContentSha256: hash, Size: 3, ResourceKey: `["attachment","a","one"]`, FetchedAt: &now},
		{Path: "files/" + hash, ContentSha256: hash, Size: 3, ResourceKey: `["attachment","a","two"]`},
	}}
	if err := b.indexOrigins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reuseDecision(b.detail, b, now, reuseAgeLimit); got != "age" {
		t.Fatalf("decision %q", got)
	}
}

func TestFetch_AmbiguousFreshFilesSkipCache(t *testing.T) {
	a, b := entry("a", "a"), entry("a", "b")
	for _, e := range []map[string]any{a, b} {
		e["size"] = 3
		e["updatedAt"] = "2026-04-14 14:41:19"
	}
	f := &fixtureServer{files: []map[string]any{a, b}, bodies: map[string]string{"a": "one", "b": "two"}}
	s, env := setup(t, f)
	s.WithCache(cacheFixture(t, "one"))
	out := process(t, s, env)
	if got := f.downloads.Load(); got != 2 {
		t.Fatalf("downloads = %d, want 2", got)
	}
	for _, body := range []string{"one", "two"} {
		data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "files", digest([]byte(body))))
		if err != nil || string(data) != body {
			t.Fatalf("body %q: %q, %v", body, data, err)
		}
	}
}
