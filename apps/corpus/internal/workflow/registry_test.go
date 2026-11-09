package workflow

import (
	"reflect"
	"testing"
)

func TestRegistry_StageOrder(t *testing.T) {
	got := Default(Deps{}).Names()
	want := []string{"discover", "fetch", "extract", "relate", "chunk", "embed", "build", "eval"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stage order = %v", got)
	}
}
