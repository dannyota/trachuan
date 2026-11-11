package expand

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestExpand_AcquisitionChecksDetailAndObservationIdentity(t *testing.T) {
	verifier, ok := any(New(nil)).(interface {
		VerifyAcquisition(context.Context, stage.Env, stage.Run, stage.Item, artifact.Result) error
	})
	if !ok {
		t.Fatal("acquisition identity verification missing")
	}
	for _, mode := range []string{"valid", "wrong detail", "wrong observation"} {
		t.Run(mode, func(t *testing.T) {
			env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
			id, seed := "1", "vn:vbpl:1"
			if mode == "wrong detail" {
				id = "2"
			}
			if mode == "wrong observation" {
				seed = "vn:vbpl:2"
			}
			art, err := env.Artifacts.PublishContent(context.Background(), "expand", stage.RecipeHash("expand", "test"), func(dir string) error {
				for name, body := range map[string]string{"detail.json": `{"data":{"id":"` + id + `","references":[]}}`, "diagram.json": `{"data":{"documentNamesByType":{}}}`, "observation.json": `{"version":1,"seed_key":"` + seed + `","targets":[],"number_only":[],"unknown_codes":[]}`} {
					if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
						return e
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			err = verifier.VerifyAcquisition(context.Background(), env, stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceList: []byte(`{"country":"vn"}`)}}, stage.Item{Key: "vn:vbpl:1"}, art)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("verification error=%v", err)
			}
		})
	}
}
