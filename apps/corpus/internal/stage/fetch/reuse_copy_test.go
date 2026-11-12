package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func reviewEntry(name, path string, size int) map[string]any {
	e := entry(name, path)
	e["size"] = size
	e["updatedAt"] = "2026-04-14 14:41:19"
	return e
}

func TestFetch_AmbiguousBaselineFilesPreserveBytes(t *testing.T) {
	for _, mode := range []string{"baseline", "fresh"} {
		t.Run(mode, func(t *testing.T) {
			firstFiles := []map[string]any{reviewEntry("a", "a", 3)}
			if mode == "baseline" {
				firstFiles = append(firstFiles, reviewEntry("a", "b", 3))
			}
			f := &fixtureServer{files: firstFiles, bodies: map[string]string{"a": "one", "b": "two"}}
			s, env := setup(t, f)
			now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
			env.Now = func() time.Time { return now }
			first := process(t, s, env)
			base := baselineFromOutput(t, env, first)
			s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
				return base, false, nil
			}
			f.files = []map[string]any{reviewEntry("a", "a", 3), reviewEntry("a", "b", 3)}
			f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null}}`
			f.downloads.Store(0)
			out := process(t, s, env)
			if f.downloads.Load() != 2 {
				t.Errorf("downloads = %d, want 2", f.downloads.Load())
			}
			var files []file
			data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "files.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(data, &files); err != nil {
				t.Fatal(err)
			}
			if len(files) != 2 {
				t.Fatalf("files: %+v", files)
			}
			got := map[string]bool{}
			for _, v := range files {
				b, e := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "files", v.SHA256))
				if e != nil {
					t.Fatal(e)
				}
				got[string(b)] = true
			}
			if !got["one"] || !got["two"] {
				t.Fatalf("attachment bytes = %v", got)
			}
		})
	}
}

func TestFetch_FullReadUsesFreshAttachmentIdentity(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{reviewEntry("a", "a", 3)}, bodies: map[string]string{"a": "one", "b": "one"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	base := baselineFromOutput(t, env, first)
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		return base, false, nil
	}
	f.files = []map[string]any{reviewEntry("a", "b", 3), reviewEntry("a", "a", 4)}
	f.bodies["a"] = "four"
	f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null}}`
	out, err := s.Process(context.Background(), env, stage.Run{}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil {
		t.Fatal(err)
	}
	adoptedCount, networkCount := 0, 0
	for _, o := range out.Observations {
		if o.Name == nil {
			continue
		}
		if o.URL == nil {
			t.Fatal("missing URL")
		}
		key, _ := json.Marshal([3]any{"attachment", *o.Name, o.URL})
		if o.ResourceKey != string(key) {
			t.Errorf("key %s, want %s", o.ResourceKey, key)
		}
		switch o.Acquisition {
		case "adopted":
			adoptedCount++
			if filepath.Base(*o.URL) != "b" {
				t.Errorf("adopted URL %s", *o.URL)
			}
		case "network":
			networkCount++
		}
	}
	if adoptedCount != 0 || networkCount != 2 {
		t.Fatalf("adopted %d network %d", adoptedCount, networkCount)
	}
}

func TestReuseDecision_ManyFiles(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	b := &baseline{detail: []byte(`{"data":{"updatedDate":"x"}}`), tree: []byte("tree"), observations: []store.RunFetchObservation{{Path: "tree.json", ContentSha256: digest([]byte("tree")), FetchedAt: &now}}}
	for i := 0; i < 3000; i++ {
		name := fmt.Sprintf("file-%d", i)
		hash := digest([]byte(name))
		b.files = append(b.files, file{Name: name, SHA256: hash, Size: int64(len(name))})
		key, _ := json.Marshal([3]any{"attachment", name, nil})
		b.observations = append(b.observations, store.RunFetchObservation{ResourceKey: string(key), Path: "files/" + hash, ContentSha256: hash, Size: int64(len(name)), FetchedAt: &now})
	}
	if err := b.indexOrigins(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, f := range b.files {
		if len(b.index.files[fileIdentity{f.Name, f.Size}]) != 1 || len(b.originsFor(f)) != 1 {
			t.Fatalf("missing indexed file %s", f.Name)
		}
	}
	if got := reuseDecision(b.detail, b); got != "reused" {
		t.Fatalf("decision %s", got)
	}
	b.observations[len(b.observations)-1].FetchedAt = nil
	if err := b.indexOrigins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reuseDecision(b.detail, b); got != "reused" {
		t.Fatalf("decision %s", got)
	}
}

func TestBaselineIndex_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &baseline{files: []file{{Name: "a", SHA256: digest([]byte("a")), Size: 1}}}
	if err := b.indexOrigins(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("index error = %v", err)
	}
}

func TestReuseDecision_ZeroOriginIsReusable(t *testing.T) {
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	zero := time.Time{}
	hash := digest([]byte("a"))
	b := &baseline{detail: []byte(`{"data":{"updatedDate":"x"}}`), tree: []byte("tree"), files: []file{{Name: "a", SHA256: hash, Size: 1}}, observations: []store.RunFetchObservation{
		{Path: "tree.json", ContentSha256: digest([]byte("tree")), FetchedAt: &zero},
		{Path: "files/" + hash, ContentSha256: hash, Size: 1, ResourceKey: `["attachment","a",null]`, FetchedAt: &now},
	}}
	if err := b.indexOrigins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reuseDecision(b.detail, b); got != "reused" {
		t.Fatalf("decision %q", got)
	}
}
