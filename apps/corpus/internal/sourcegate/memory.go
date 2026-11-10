package sourcegate

import (
	"context"
	"sync"
)

type sourceKey struct {
	runID  int64
	source string
}

// Memory is an in-memory gate for client unit tests.
type Memory struct {
	mu     sync.Mutex
	paused map[sourceKey]bool
}

// NewMemory constructs an in-memory gate.
func NewMemory() *Memory { return &Memory{paused: make(map[sourceKey]bool)} }

// Admit checks whether the source is paused.
func (g *Memory) Admit(ctx context.Context, runID int64, source string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused[sourceKey{runID, source}] {
		return ErrSourcePaused
	}
	return nil
}

// Pause marks the source as paused.
func (g *Memory) Pause(ctx context.Context, runID int64, source, _, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paused[sourceKey{runID, source}] = true
	return nil
}

// Clear explicitly restores admission for the source.
func (g *Memory) Clear(ctx context.Context, runID int64, source, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.paused, sourceKey{runID, source})
	return nil
}
