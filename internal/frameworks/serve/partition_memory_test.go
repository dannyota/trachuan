package serve

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartitionMemory_RejectsReleaseOverLimit(t *testing.T) {
	e := &Engine{}
	require.NoError(t, e.reservePartitionMemory(256*1024*1024-1))
	require.NoError(t, e.reservePartitionMemory(1))
	require.ErrorIs(t, e.reservePartitionMemory(1), ErrUnavailable)
	require.Equal(t, int64(256*1024*1024), e.partitionBytes)
	for _, bytes := range []int64{-1, math.MaxInt64} {
		require.ErrorIs(t, e.reservePartitionMemory(bytes), ErrUnavailable)
	}
}
