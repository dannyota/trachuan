package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/pinned"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestProcess_PinnedBundleBypassesSource(t *testing.T) {
	ctx := context.Background()
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	attachment := []byte("stored attachment")
	hash := digest(attachment)
	attachmentURL := "https://example.test/attachment.pdf"
	attachmentKey, _ := json.Marshal([3]any{"attachment", "stored.pdf", attachmentURL})
	stamp := time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC)
	files := map[string][]byte{"detail.json": []byte(`{"data":{"id":"pin","docNum":"71/2099/QH15"}}`), "diagram.json": []byte(`{"data":[]}`), "tree.json": []byte(`{"data":[]}`), "files.json": []byte(`[]`), "report.json": []byte(`{"missing":["missing.pdf"],"too_large":["large.pdf"]}`)}
	files["files.json"] = []byte(fmt.Sprintf(`[{"name":"stored.pdf","sha256":"%s","size":%d}]`, hash, len(attachment)))
	files["files/"+hash] = attachment
	art, err := env.Artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
		for name, data := range files {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pin := source.Pin{Key: "vn:vbpl:pin", Number: "71/2099/QH15", Fetch: art.SHA256, Expand: "expand", Rel: "20990101_1", Since: "2099-01-02"}
	raw, _ := json.Marshal(source.List{VBPL: source.VBPLSources{Pinned: []source.Pin{pin}}})
	r := stage.Run{Country: "vn", Rel: "20990102_1", Snapshot: stage.Snapshot{SourceList: raw}}
	for n := 1; !controlSample(pin.Key, r.Rel); n++ {
		r.Rel = fmt.Sprintf("20990102_%d", n)
	}
	s := New(nil)
	s.loadPinned = func(context.Context, stage.Env, stage.Run, source.Pin) (pinned.Acquisition, error) {
		return pinned.Acquisition{Artifact: store.GetPinnedArtifactRow{RunID: 1, Generation: 1, Attempt: 1, Path: art.Path, Sha256: art.SHA256}, Observations: []store.RunFetchObservation{
			{RunID: 1, ItemKey: pin.Key, Generation: 1, Attempt: 1, ArtifactSha256: art.SHA256, ResourceKey: string(attachmentKey), Source: "vbpl", Path: "files/" + hash, ContentSha256: hash, Size: int64(len(attachment)), Url: &attachmentURL, FetchedAt: &stamp},
		}}, nil
	}
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		t.Fatal("pin selected a baseline")
		return nil, false, nil
	}
	s.loadExpand = func(context.Context, stage.Env, stage.Run, stage.Item) ([]byte, []byte, bool, error) {
		t.Fatal("pin entered the live read/control path")
		return nil, nil, false, nil
	}
	out, err := s.Process(ctx, env, r, stage.Item{Key: pin.Key})
	if err != nil {
		t.Fatal(err)
	}
	if out.Artifact == nil || out.Artifact.SHA256 != art.SHA256 || len(out.Sources) != 0 {
		t.Fatalf("output=%+v", out)
	}
	for name, want := range files {
		got, readErr := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), name))
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("file %s differs: %v", name, readErr)
		}
	}
	if len(out.Observations) != 4 {
		t.Fatalf("observations=%+v", out.Observations)
	}
	for _, o := range out.Observations {
		if o.Name != nil {
			if o.OriginRef == nil || o.FetchedAt == nil || !o.FetchedAt.Equal(stamp) || o.URL == nil || *o.URL != attachmentURL {
				t.Fatalf("lost attachment origin: %+v", o)
			}
		} else if o.FetchedAt != nil {
			t.Fatalf("unknown origin acquired a time: %+v", o)
		}
		if o.Acquisition != "pinned" {
			t.Fatalf("observation=%+v", o)
		}
	}
	res, err := s.Resolve(ctx, env, r, stage.Item{Key: pin.Key}, *out.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, gap := range res.Gaps {
		kinds[gap.Kind] = true
	}
	for _, kind := range []string{"missing_attachment", "file_too_large", "source_withdrawn_retained"} {
		if !kinds[kind] {
			t.Errorf("missing %s: %+v", kind, res.Gaps)
		}
	}
}

func TestPinned_ContextStillReadsSource(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{}, detailBody: `{"success":true,"data":{"id":"66518","docNum":"72/2099/QH15"}}`}
	s, env := setup(t, f)
	raw, err := json.Marshal(source.List{VBPL: source.VBPLSources{Pinned: []source.Pin{{Key: "vn:vbpl:pin"}}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn", Rel: "20990102_1", Snapshot: stage.Snapshot{SourceList: raw}}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil || out.State != "done" || f.requests.Load() != 4 {
		t.Fatalf("context state=%s calls=%d gaps=%+v err=%v", out.State, f.requests.Load(), out.Gaps, err)
	}
}
