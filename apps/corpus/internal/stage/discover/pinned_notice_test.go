package discover

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestPinned_LiveNoticeWithoutGap(t *testing.T) {
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	rules := source.VBPLSources{Pinned: []source.Pin{{Key: "vn:vbpl:pin"}}}
	for _, complete := range []bool{false, true} {
		notices := livePins([]string{"pin"}, rules.Pinned, complete)
		if len(notices) != map[bool]int{false: 0, true: 1}[complete] {
			t.Fatalf("complete=%v notices=%+v", complete, notices)
		}
		art, err := env.Artifacts.PublishContent(context.Background(), "discover", stage.RecipeHash("test", "pin"), func(dir string) error {
			result := scopeReport{Version: 2, Stable: true, QueryIdentity: source.CentralListingIdentity("vbpl", "TRUNG_UONG", 1000), Notices: notices}
			for name, value := range map[string]any{"report.json": result, "selected.json": selectedFile{}} {
				b, _ := json.Marshal(value)
				if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := resolveScope(env, stage.Run{}, stage.Item{Key: "vbpl:central"}, art, rules)
		if err != nil || len(res.Gaps) != 0 {
			t.Fatalf("resolution=%+v err=%v", res, err)
		}
	}
}
