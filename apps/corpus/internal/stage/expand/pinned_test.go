package expand

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestProcess_PinKeepsContextWithoutSource(t *testing.T) {
	ctx := context.Background()
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	detail := []byte(`{"data":{"id":"pin","docNum":"71/2099/QH15","references":[{"referenceType":"1","targetDocument":{"id":"context"}}]}}`)
	diagram := []byte(`{"data":{}}`)
	old, err := env.Artifacts.PublishContent(ctx, "expand", strings.Repeat("a", 64), func(dir string) error {
		for name, data := range map[string][]byte{"detail.json": detail, "diagram.json": diagram, "observation.json": []byte(`{}`)} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pin := source.Pin{Key: "vn:vbpl:pin", Expand: old.SHA256}
	list := source.List{RelationCodes: map[string]source.RelationCode{"1": {Kind: "repeals"}}, VBPL: source.VBPLSources{Pinned: []source.Pin{pin}}}
	raw, _ := json.Marshal(list)
	s := New(nil)
	s.loadPinned = func(context.Context, stage.Env, stage.Run, source.Pin) (store.GetPinnedArtifactRow, error) {
		return store.GetPinnedArtifactRow{Path: old.Path, Sha256: old.SHA256}, nil
	}
	out, err := s.Process(ctx, env, stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceList: raw}}, stage.Item{Key: pin.Key})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "done" || len(out.Sources) != 0 || out.Artifact == nil {
		t.Fatalf("output=%+v", out)
	}
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "observation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var observed Observation
	if err = json.Unmarshal(data, &observed); err != nil {
		t.Fatal(err)
	}
	if len(observed.Targets) != 1 || observed.Targets[0].TargetKey != "vn:vbpl:context" {
		t.Fatalf("context=%+v", observed)
	}
	if err = s.VerifyAcquisition(ctx, env, stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceList: raw}}, stage.Item{Key: pin.Key}, *out.Artifact); err != nil {
		t.Fatal(err)
	}
}
