package cli

import (
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/workflow"
)

func compiledStages(snapshot stage.Snapshot) ([]string, map[string]string) {
	// Registered stages need no dependencies beyond Env; names and recipes do not need a pool.
	registry := workflow.Default(workflow.Deps{Env: stage.Env{}})
	return registry.Names(), registry.Recipes(snapshot)
}
