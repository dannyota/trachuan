package sourcegate

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type failedDB struct{}

func (failedDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("database unavailable")
}

func TestSourceGate_FailsClosedOnDBError(t *testing.T) {
	g := New(failedDB{})
	if err := g.Admit(context.Background(), 1, "vbpl"); err == nil || errors.Is(err, ErrSourcePaused) {
		t.Fatalf("admit error = %v", err)
	}
}

func TestMemoryGate_PauseAndClear(t *testing.T) {
	ctx := context.Background()
	g := NewMemory()
	if err := g.Admit(ctx, 1, "vbpl"); err != nil {
		t.Fatal(err)
	}
	if err := g.Pause(ctx, 1, "vbpl", "challenge", "path hash"); err != nil {
		t.Fatal(err)
	}
	if err := g.Admit(ctx, 1, "vbpl"); !errors.Is(err, ErrSourcePaused) {
		t.Fatalf("admit = %v", err)
	}
	if err := g.Admit(ctx, 2, "vbpl"); err != nil {
		t.Fatal(err)
	}
	if err := g.Clear(ctx, 1, "vbpl", "owner note"); err != nil {
		t.Fatal(err)
	}
	if err := g.Admit(ctx, 1, "vbpl"); err != nil {
		t.Fatal(err)
	}
}
