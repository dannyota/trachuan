package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandle_CanonicalVersionAndControl(t *testing.T) {
	h, err := parseHandle("fw:fw-alpha:2#A.5%20%2F%20B(1)")
	require.NoError(t, err)
	require.Equal(t, "fw-alpha", h.code)
	require.Equal(t, "2", h.version)
	require.Equal(t, "A.5 / B(1)", h.citation)
	h, err = parseHandle("fw:fw-alpha:2")
	require.NoError(t, err)
	require.Empty(t, h.citation)
	for _, raw := range []string{"", "fw:FW-ALPHA:2", "fw:fw-alpha:2#", "fw:fw-alpha:2#A%2fB", "fw:fw-alpha:2#%41", "fw:fw-alpha:2#A B", "fw:fw-alpha:2#%00", "fw:fw-alpha:2#%FF", "fw:fw-alpha:2#A#B", "vn:law#article"} {
		_, err = parseHandle(raw)
		require.ErrorIs(t, err, ErrInvalidHandle, raw)
	}
}
