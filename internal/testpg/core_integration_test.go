//go:build integration

package testpg

import (
	"context"
	"testing"
)

func TestCore_CreateAndDropMarkedCorpus(t *testing.T) {
	ctx := context.Background()
	conn, _ := administrator(t)
	name, key, err := CreateCorpus(ctx, conn, "fixture core")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { DropCorpusDatabase(t, conn, name, key) })
	foreign := "vn/20990101"
	if key == foreign {
		foreign = "vn/20990102"
	}
	dropped, err := DropMarkedCorpus(ctx, conn, name, foreign)
	if err != nil {
		t.Fatal(err)
	}
	if dropped {
		t.Fatal("accepted a foreign marker")
	}
	dropped, err = DropMarkedCorpus(ctx, conn, name, key)
	if err != nil || !dropped {
		t.Fatalf("drop: %t %v", dropped, err)
	}
}
