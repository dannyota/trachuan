//go:build integration

package serve

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

type operationTwin struct {
	a, b           *Engine
	traceA, traceB *queryTrace
	caller         Caller
}

func operationBytes(t *testing.T, value any, err error) []byte {
	t.Helper()
	message := ""
	if err != nil {
		message = err.Error()
	}
	raw, marshalErr := json.Marshal(struct {
		Result any    `json:"result"`
		Error  string `json:"error,omitempty"`
	}{value, message})
	require.NoError(t, marshalErr)
	return raw
}

func (p operationTwin) reset() {
	p.traceA.reset()
	p.traceB.reset()
}

func (p operationTwin) compare(t *testing.T, a, b any, ea, eb error) {
	t.Helper()
	require.Equal(t, operationBytes(t, a, ea), operationBytes(t, b, eb))
	p.traceA.assertProjection(t, p.a.project(p.caller).readable)
	p.traceB.assertProjection(t, p.b.project(p.caller).readable)
	require.Equal(t, p.traceA.snapshot(), p.traceB.snapshot())
}

func TestOperations_UnreadableTextInvariant(t *testing.T) {
	a, traceA := buildOperationsFixture(t, false)
	b, traceB := buildOperationsFixture(t, true)
	for name, caller := range map[string]Caller{"empty": {}, "reduced": fakeCaller(), "mixed": fakeCaller("fw-gamma")} {
		t.Run(name, func(t *testing.T) {
			pair := operationTwin{a, b, traceA, traceB, caller}
			assertStatusInvariant(t, pair)
			assertMappingsInvariant(t, pair)
			assertReadInvariant(t, pair)
			assertGapsInvariant(t, pair)
		})
	}
	t.Run("entitled positive control", func(t *testing.T) {
		ctx := context.Background()
		caller := fakeCaller("fw-alpha")
		traceA.reset()
		traceB.reset()
		ra, ea := a.Read(ctx, ReadRequest{Items: []ReadItem{{Handle: "fw:fw-alpha:2#A-1"}}}, caller)
		rb, eb := b.Read(ctx, ReadRequest{Items: []ReadItem{{Handle: "fw:fw-alpha:2#A-1"}}}, caller)
		require.NoError(t, ea)
		require.NoError(t, eb)
		require.NotEqual(t, operationBytes(t, ra, ea), operationBytes(t, rb, eb))
		ma, ea := a.Mappings(ctx, MappingsRequest{Handle: "fw:fw-alpha:2#A-1", Limit: 1}, caller)
		mb, eb := b.Mappings(ctx, MappingsRequest{Handle: "fw:fw-alpha:2#A-1", Limit: 1}, caller)
		require.NoError(t, ea)
		require.NoError(t, eb)
		require.Len(t, ma.Edges, 1)
		require.Len(t, mb.Edges, 1)
		require.NotEqual(t, ma.Edges[0].Evidence, mb.Edges[0].Evidence)
		require.NotNil(t, ma.Edges[0].EvidenceTruncated)
		require.LessOrEqual(t, len([]rune(ma.Edges[0].Evidence)), 2000)
		sa, ea := a.CorpusStatus(ctx, StatusRequest{}, caller)
		sb, eb := b.CorpusStatus(ctx, StatusRequest{}, caller)
		require.NoError(t, ea)
		require.NoError(t, eb)
		require.NotEqual(t, sa.Versions["fw:fw-alpha:2"].Sources, sb.Versions["fw:fw-alpha:2"].Sources)
		ga, ea := a.QualityGaps(ctx, GapsRequest{Versions: []string{"fw:fw-alpha:2"}, Kind: "text_incomplete"}, caller)
		gb, eb := b.QualityGaps(ctx, GapsRequest{Versions: []string{"fw:fw-alpha:2"}, Kind: "text_incomplete"}, caller)
		require.NoError(t, ea)
		require.NoError(t, eb)
		require.Len(t, ga.Gaps, 1)
		require.Len(t, gb.Gaps, 1)
		require.NotEqual(t, ga.Gaps[0].Detail, gb.Gaps[0].Detail)
		assertEntitledRead(t, a, caller, "quartz", 1500)
		assertEntitledRead(t, b, caller, "nebula", 2100)
		traceA.assertProjection(t, a.project(caller).readable)
		traceB.assertProjection(t, b.project(caller).readable)
	})
}

func assertStatusInvariant(t *testing.T, pair operationTwin) {
	t.Helper()
	pair.reset()
	a, ea := pair.a.CorpusStatus(context.Background(), StatusRequest{}, pair.caller)
	b, eb := pair.b.CorpusStatus(context.Background(), StatusRequest{}, pair.caller)
	pair.compare(t, a, b, ea, eb)
	require.NoError(t, ea)
	require.Equal(t, "placeholder", a.RetrievalRecipeStatus)
	require.Len(t, a.Versions, 4)
	require.Equal(t, Counts{Controls: 16, EdgesResolved: 206, EdgesUnresolved: 1}, a.Totals)
	require.Equal(t, 205, a.Versions["fw:fw-alpha:2"].EdgesResolved)
	require.Equal(t, 1, a.Versions["fw:fw-beta:1"].EdgesResolved)
	require.Equal(t, 1, a.Versions["fw:fw-gamma:1"].EdgesUnresolved)
	require.Empty(t, a.Versions["fw:fw-alpha:2"].Sources)
	for _, version := range a.Versions {
		if !version.Readable {
			require.Empty(t, version.Sources)
		}
	}
}

func assertGapsInvariant(t *testing.T, pair operationTwin) {
	t.Helper()
	for _, request := range []GapsRequest{{}, {Versions: []string{"fw:fw-alpha:2"}}, {Kind: "mapping_dropped"}, {Kind: "text_incomplete"}, {Kind: "unknown"}, {Versions: []string{"fw:fw-alpha:99"}}} {
		var previous []string
		total := 0
		for page := 0; ; page++ {
			require.Less(t, page, 5)
			pair.reset()
			a, ea := pair.a.QualityGaps(context.Background(), request, pair.caller)
			b, eb := pair.b.QualityGaps(context.Background(), request, pair.caller)
			pair.compare(t, a, b, ea, eb)
			if request.Kind == "unknown" || len(request.Versions) > 0 && request.Versions[0] == "fw:fw-alpha:99" {
				require.ErrorIs(t, ea, ErrInvalidRequest)
				break
			}
			require.NoError(t, ea)
			require.Equal(t, "placeholder", a.RetrievalRecipeStatus)
			require.LessOrEqual(t, len(a.Gaps), 200)
			for _, gap := range a.Gaps {
				require.Equal(t, "mapping_dropped", gap.Kind)
				key := []string{gap.Kind, gap.Version, gap.Subject, gap.Detail}
				if previous != nil {
					require.Positive(t, slices.Compare(key, previous))
				}
				previous = key
			}
			total += len(a.Gaps)
			if a.NextCursor == "" {
				if request.Kind == "text_incomplete" {
					require.Zero(t, total)
				} else {
					require.Equal(t, 205, total)
				}
				break
			}
			require.Len(t, a.Gaps, 200)
			request.Cursor = a.NextCursor
		}
	}
}
