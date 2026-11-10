package fetch

import (
	"context"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
)

func TestHandOverReader_SecondaryBundle(t *testing.T) {
	token := "abc"
	s, env := secondaryFixture(t, false, &token)
	out := secondaryFetch(t, s, env)
	row := store.ListStageItemArtifactsRow{ItemKey: "vn:chinhphu:218029", State: "done", Path: &out.Artifact.Path, Sha256: &out.Artifact.SHA256}
	got, refs, err := s.readSecondaryHandOver(context.Background(), env, row)
	if err != nil || got.number != "142/2026/ND-CP" || got.issuer == "" || got.date != "2026-04-30" || len(refs) != 3 {
		t.Fatalf("identity=%+v refs=%v err=%v", got, refs, err)
	}
	if _, _, err := New(nil).readSecondaryHandOver(context.Background(), env, row); err == nil {
		t.Fatal("accepted missing parser")
	}
}

func TestHandOverReader_VBPLVerifiesArtifact(t *testing.T) {
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	art, err := env.Artifacts.PublishContent(context.Background(), "fetch", stage.RecipeHash("test", "vbpl"), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "detail.json"), []byte(`{"data":{"id":"1","docNum":"42","agencyName":"Government","issueDate":"2026-01-01"}}`), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	row := store.ListStageItemArtifactsRow{ItemKey: "vn:vbpl:1", State: "done", Path: &art.Path, Sha256: &art.SHA256}
	got, refs, err := New(nil).readVBPLHandOver(context.Background(), env, row)
	if err != nil || got.number != "42" || got.issuer != "GOVERNMENT" || got.date != "2026-01-01" || len(refs) != 1 {
		t.Fatalf("identity=%+v refs=%v err=%v", got, refs, err)
	}
	if err = os.WriteFile(filepath.Join(env.Artifacts.Abs(art.Path), "detail.json"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = New(nil).readVBPLHandOver(context.Background(), env, row); err == nil {
		t.Fatal("accepted modified detail")
	}
}
