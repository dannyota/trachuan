package build

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

func TestLoad_RejectsNilConnection(t *testing.T) {
	_, err := Load(context.Background(), nil, Inventory{}, artifact.Store{}, Recipes{}, nil)
	if err == nil {
		t.Fatal("expected connection error")
	}
}
