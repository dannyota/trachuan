// Package run implements corpus run lifecycle operations.
package run

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func validateRel(rel string) error {
	matched, err := regexp.MatchString(`^[0-9]{8}(_[0-9]{1,2})?$`, rel)
	if err != nil {
		return fmt.Errorf("validate release: %w", err)
	}
	if !matched {
		return fmt.Errorf("invalid release %q", rel)
	}
	return nil
}

// CompareRel orders releases by date, then by sequence number as an integer.
func CompareRel(a, b string) int {
	ap := strings.SplitN(a, "_", 2)
	bp := strings.SplitN(b, "_", 2)
	if ap[0] != bp[0] {
		return strings.Compare(ap[0], bp[0])
	}
	var ai, bi int
	if len(ap) == 2 {
		ai, _ = strconv.Atoi(ap[1])
	}
	if len(bp) == 2 {
		bi, _ = strconv.Atoi(bp[1])
	}
	if ai < bi {
		return -1
	}
	if ai > bi {
		return 1
	}
	return 0
}

func extendSnapshot(snapshot stage.Snapshot, stages []string, recipes map[string]string) (stage.Snapshot, bool, error) {
	for _, name := range snapshot.Stages {
		recipe, ok := recipes[name]
		if !ok || snapshot.Recipes[name] != recipe {
			return stage.Snapshot{}, false, fmt.Errorf("frozen recipe differs for stage %q", name)
		}
	}
	result := snapshot
	result.Stages = append([]string(nil), snapshot.Stages...)
	result.Recipes = make(map[string]string, len(snapshot.Recipes))
	for name, recipe := range snapshot.Recipes {
		result.Recipes[name] = recipe
	}
	changed := false
	for _, name := range stages {
		if _, ok := result.Recipes[name]; ok {
			continue
		}
		recipe, ok := recipes[name]
		if !ok {
			return stage.Snapshot{}, false, fmt.Errorf("missing recipe for stage %q", name)
		}
		result.Stages = append(result.Stages, name)
		result.Recipes[name] = recipe
		changed = true
	}
	return result, changed, nil
}
