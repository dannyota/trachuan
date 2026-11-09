package build

import (
	"context"
	"reflect"
	"testing"

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
