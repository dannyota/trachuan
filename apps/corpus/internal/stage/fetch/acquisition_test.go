package fetch

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestFetch_AcquisitionChecksDocumentIdentityAndFormat(t *testing.T) {
	for _, secondary := range []bool{false, true} {
		var s *Stage
		var env stage.Env
		var out stage.Output
		key := "vn:vbpl:66518"
		if secondary {
			token := "placeholder"
			s, env = secondaryFixture(t, false, &token)
			out = secondaryFetch(t, s, env)
			key = "vn:chinhphu:218029"
		} else {
			s, env = setup(t, &fixtureServer{})
			out = process(t, s, env)
		}
		verifier, ok := any(s).(interface {
			VerifyAcquisition(context.Context, stage.Env, stage.Run, stage.Item, artifact.Result) error
		})
		if !ok {
			t.Fatal("acquisition identity verification missing")
		}
		if err := verifier.VerifyAcquisition(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: key}, *out.Artifact); err != nil {
			t.Fatal(err)
		}
		for _, wrong := range []string{"vn:vbpl:wrong", "vn:chinhphu:wrong"} {
			if err := verifier.VerifyAcquisition(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: wrong}, *out.Artifact); err == nil {
				t.Fatalf("adopted %s as %s", key, wrong)
			}
		}
	}
}
