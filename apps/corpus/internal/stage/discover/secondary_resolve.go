package discover

import (
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func resolveSecondary(env stage.Env, it stage.Item, art artifact.Result) (stage.Resolution, error) {
	var report secondaryReport
	if err := readJSON(env.Artifacts.Abs(art.Path), "secondary.json", &report); err != nil {
		return stage.Resolution{}, err
	}
	if report.Version != 1 {
		return stage.Resolution{}, fmt.Errorf("unsupported secondary artifact version %d", report.Version)
	}
	if report.Definition.Key != it.Key || report.QueryIdentity != secondaryIdentity(report.Definition) {
		return stage.Resolution{}, fmt.Errorf("secondary query identity mismatch")
	}
	return stage.Resolution{Gaps: report.Gaps}, nil
}
func (s *Stage) candidateCeiling() int {
	if s.ceiling > 0 {
		return s.ceiling
	}
	return 100
}
