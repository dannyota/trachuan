package fetch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

// VerifyAcquisition checks the expected source format and document identity.
// The caller has already verified the whole bundle and its recorded hash.
func (s *Stage) VerifyAcquisition(ctx context.Context, env stage.Env, r stage.Run, item stage.Item, art artifact.Result) error {
	dir := env.Artifacts.Abs(art.Path)
	if strings.HasPrefix(item.Key, "vn:vbpl:") {
		if _, err := os.Stat(filepath.Join(dir, "document.json")); err == nil {
			return fmt.Errorf("mixed fetch formats")
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect fetch format: %w", err)
		}
	}
	if _, err := ReadDocument(dir, item.Key); err != nil {
		return fmt.Errorf("verify acquisition document: %w", err)
	}
	if _, err := s.Resolve(ctx, env, r, item, art); err != nil {
		return fmt.Errorf("verify acquisition report: %w", err)
	}
	return nil
}
