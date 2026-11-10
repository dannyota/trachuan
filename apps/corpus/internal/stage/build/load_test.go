package build

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

func TestLoad_RejectsNilConnection(t *testing.T) {
	_, err := Load(context.Background(), nil, Inventory{}, artifact.Store{}, Recipes{}, nil, nil, "2099-01-01")
	if err == nil {
		t.Fatal("expected connection error")
	}
}
