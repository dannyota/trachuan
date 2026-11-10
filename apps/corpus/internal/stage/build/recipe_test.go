package build

import (
	"context"
	"reflect"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/evaluation"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestBuildRecipe_EveryFieldAffectsHash(t *testing.T) {
	r := buildRecipe{}
	base := r.hash()
	v := reflect.ValueOf(&r).Elem()
	for i := 0; i < v.NumField(); i++ {
		x := v.Field(i)
		switch x.Kind() {
		case reflect.String:
			x.SetString("changed")
		case reflect.Int:
			x.SetInt(42)
		default:
			t.Fatalf("unhandled field %s", v.Type().Field(i).Name)
		}
		if r.hash() == base {
			t.Errorf("field %s did not change hash", v.Type().Field(i).Name)
		}
		x.Set(reflect.Zero(x.Type()))
	}
}

func TestPlan_RejectsYearZero(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("year zero reached workshop: %v", r)
		}
	}()
	_, err := New(Config{}).Plan(context.Background(), stage.Env{}, stage.Run{Country: "vn", Rel: "00000101"})
	if err == nil {
		t.Fatal("accepted year zero")
	}
}

func TestBuildInput_MapBytesChangeInputNotRecipe(t *testing.T) {
	first := buildInputHash("vn", "20990101", "manifest", "map-one")
	second := buildInputHash("vn", "20990101", "manifest", "map-two")
	if first == second {
		t.Fatal("map did not change input hash")
	}
	snapshot := stage.Snapshot{SourceList: []byte(`{"country":"vn","status_codes":{}}`)}
	a := New(Config{Suite: evaluation.Suite{MapBytes: []byte("map-one"), Hashes: evaluation.FixtureHashes{IdentityMap: "map-one"}}}).Recipe(snapshot)
	b := New(Config{Suite: evaluation.Suite{MapBytes: []byte("map-two"), Hashes: evaluation.FixtureHashes{IdentityMap: "map-two"}}}).Recipe(snapshot)
	if a == "" || a != b {
		t.Fatalf("map changed build recipe: %q != %q", a, b)
	}
}

func TestBuild_InitialEvaluationSummary(t *testing.T) {
	if got := initialEvaluationSummary(); string(got) != `{"status":"not_run","release_eligible":false}` {
		t.Fatalf("summary = %s", got)
	}
}
