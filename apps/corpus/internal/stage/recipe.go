package stage

import "fmt"

// RecipeError refuses work whose compiled recipe does not match the frozen run.
type RecipeError struct {
	Stage   string
	Missing bool
}

// Error returns the same refusal for planning and batch processing.
func (e *RecipeError) Error() string {
	if e.Missing {
		return fmt.Sprintf("missing frozen recipe for stage %s", e.Stage)
	}
	return fmt.Sprintf("recipe mismatch for stage %s", e.Stage)
}

func checkRecipe(snapshot Snapshot, name, compiled string) error {
	frozen, ok := snapshot.Recipes[name]
	if !ok {
		return &RecipeError{Stage: name, Missing: true}
	}
	if frozen != compiled {
		return &RecipeError{Stage: name}
	}
	return nil
}
