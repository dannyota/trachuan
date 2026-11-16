package build

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStepInputHash_CanonicalMap(t *testing.T) {
	a, b := "first", "second"
	for _, inputs := range []map[string]*string{
		{"b": &b, "optional": nil, "a": &a},
		{"a": &a, "b": &b, "optional": nil},
	} {
		require.Equal(t, digest([]byte(`{"a":"first","b":"second","optional":null}`)), stepInputHash(inputs))
	}
	require.Equal(t, "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", stepInputHash(map[string]*string{}))
	require.Equal(t, stepInputHash(map[string]*string{}), stepInputHash(nil))
	require.NotEqual(t, stepInputHash(map[string]*string{"optional": nil}), stepInputHash(map[string]*string{}))
}

func TestDatabaseInputs_ReadsStepRecipesAndInputHashes(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	base := filepath.Join(work, "20990101")
	d, err := readDatabaseInputs(base)
	require.NoError(t, err)
	require.Len(t, d.steps, 5)
	for step, provenance := range d.steps {
		t.Run(step, func(t *testing.T) {
			data, readErr := readFile(filepath.Join(base, step, "manifest.json"))
			require.NoError(t, readErr)
			var m manifest
			require.NoError(t, json.Unmarshal(data, &m))
			require.Equal(t, m.Recipe, provenance.Recipe)
			canonical, marshalErr := json.Marshal(m.Inputs)
			require.NoError(t, marshalErr)
			require.Equal(t, digest(canonical), provenance.InputHash)
		})
	}
}
