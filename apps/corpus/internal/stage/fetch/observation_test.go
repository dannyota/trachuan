package fetch

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFetch_BundleBytesIndependentOfClock(t *testing.T) {
	s, env := setup(t, &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}})
	first := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return first }
	a := process(t, s, env)
	names := []string{"detail.json", "tree.json", "diagram.json", "files.json", "report.json", "MANIFEST.sha256", fmt.Sprintf("files/%x", sha256.Sum256([]byte("one")))}
	contents := make(map[string][]byte)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(a.Artifact.Path), name))
		if err != nil {
			t.Fatal(err)
		}
		contents[name] = data
	}
	second := first.Add(time.Hour)
	env.Now = func() time.Time { return second }
	b := process(t, s, env)
	if a.Artifact.SHA256 != b.Artifact.SHA256 {
		t.Fatal("clock changed bundle hash")
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(b.Artifact.Path), name))
		if err != nil || !bytes.Equal(data, contents[name]) {
			t.Fatalf("bundle %s differs: %v", name, err)
		}
	}
	if len(a.Observations) != 4 || len(b.Observations) != 4 {
		t.Fatalf("observations: %d %d", len(a.Observations), len(b.Observations))
	}
	endpoints := []string{"doc/detail", "doc/provision/tree", "doc/diagram", "attachment"}
	for i := range a.Observations {
		x, y := a.Observations[i], b.Observations[i]
		var key [3]*string
		if err := json.Unmarshal([]byte(x.ResourceKey), &key); err != nil || key[0] == nil || *key[0] != endpoints[i] {
			t.Fatalf("resource key %q: %v", x.ResourceKey, err)
		}
		if i < 3 && (key[1] != nil || key[2] != nil || x.Name != nil || x.URL != nil) {
			t.Fatalf("response identity: %+v", x)
		}
		if i == 3 && (key[1] == nil || *key[1] != "a" || key[2] == nil || x.URL == nil || *key[2] != *x.URL || x.Name == nil || *x.Name != "a") {
			t.Fatalf("attachment identity: %+v", x)
		}
		if x.FetchedAt == nil || y.FetchedAt == nil || !x.FetchedAt.Equal(first) || !y.FetchedAt.Equal(second) || x.Acquisition != "network" || x.ResourceKey != y.ResourceKey {
			t.Fatalf("observations: %+v %+v", x, y)
		}
		data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(a.Artifact.Path), x.Path))
		if err != nil || x.ContentSHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || x.Size != int64(len(data)) {
			t.Fatalf("content observation %+v: %v", x, err)
		}
	}
}

func TestFetch_CacheHitAndDownloadSameBytes(t *testing.T) {
	e := entry("a", "a")
	e["size"] = 3
	e["updatedAt"] = "2026-04-14 14:41:19"
	f := &fixtureServer{files: []map[string]any{e}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	downloaded := process(t, s, env)
	s.WithCache(cacheFixture(t, "one"))
	cached := process(t, s, env)
	if downloaded.Artifact.SHA256 != cached.Artifact.SHA256 {
		t.Fatal("cache changed bundle hash")
	}
	if len(downloaded.Observations) != 4 || len(cached.Observations) != 4 {
		t.Fatal("missing observations")
	}
	d, c := downloaded.Observations[3], cached.Observations[3]
	if d.Acquisition != "network" || d.FetchedAt == nil || c.Acquisition != "cache" || c.FetchedAt != nil || c.OriginRef != nil || d.ContentSHA256 != c.ContentSHA256 {
		t.Fatalf("download %+v cache %+v", d, c)
	}
}

func TestFetch_DuplicateResourceHasOneObservation(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a"), entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	out := process(t, s, env)
	if len(out.Observations) != 4 {
		t.Fatalf("observations = %d, want four unique resources", len(out.Observations))
	}
}
