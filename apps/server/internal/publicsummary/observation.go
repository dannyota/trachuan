package publicsummary

import (
	"context"
	"fmt"
)

type observation struct {
	release  Release
	sequence uint64
	err      error
}

func (m *Manager) readMetadata(ctx context.Context) (observation, error) {
	select {
	case <-ctx.Done():
		return observation{}, ctx.Err()
	case m.metadataSlot <- struct{}{}:
	}
	result := make(chan observation, 1)
	go func() {
		defer func() { <-m.metadataSlot }()
		release, err := m.metadata(ctx)
		m.mu.Lock()
		m.sequence++
		sequence := m.sequence
		m.mu.Unlock()
		result <- observation{release: release, sequence: sequence, err: err}
	}()
	select {
	case <-ctx.Done():
		return observation{}, ctx.Err()
	case o := <-result:
		if err := ctx.Err(); err != nil {
			return observation{}, fmt.Errorf("read public metadata: %w", err)
		}
		return o, nil
	}
}
