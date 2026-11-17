package evaluate

import (
	"context"
	"testing"

	"danny.vn/trachuan/internal/frameworks/serve"
	"github.com/stretchr/testify/require"
)

type invariantFixture struct {
	changed, leak, unchanged string
}

func (f invariantFixture) value(class string, caller serve.Caller) string {
	if f.changed != "" && class != f.unchanged && (caller.Readable("fw-alpha", "licensed") || f.leak == class) {
		return "changed"
	}
	return "original"
}
func (f invariantFixture) Search(_ context.Context, r serve.Request, c serve.Caller) (serve.Response, error) {
	if r.Query == "errored diagnostic" {
		return serve.Response{}, serve.ErrUnavailable
	}
	score := 1.0
	if f.value("body_index", c) == "changed" {
		score = 2
	}
	response := serve.Response{Hits: []serve.Hit{{Handle: "fw:fw-alpha:1#A", Score: &score}}}
	if r.Cursor == "" {
		response.NextCursor = "page-two"
	}
	return response, nil
}
func (f invariantFixture) Read(_ context.Context, r serve.ReadRequest, c serve.Caller) (serve.ReadResponse, error) {
	item := serve.ReadItemResult{Complete: r.Items[0].Next != "", Units: []serve.ReadUnit{{Kind: "body", Text: f.value("control_text", c)}}}
	if !item.Complete {
		item.Next = "read-two"
	}
	return serve.ReadResponse{Items: []serve.ReadItemResult{item}}, nil
}
func (f invariantFixture) Mappings(_ context.Context, r serve.MappingsRequest, c serve.Caller) (serve.MappingsResponse, error) {
	response := serve.MappingsResponse{Edges: []serve.MappingEdge{{Evidence: f.value("mapping_evidence", c)}}}
	if r.Cursor == "" {
		response.NextCursor = "mapping-two"
	}
	return response, nil
}
func (f invariantFixture) QualityGaps(_ context.Context, r serve.GapsRequest, c serve.Caller) (serve.GapsResponse, error) {
	response := serve.GapsResponse{Gaps: []serve.Gap{{Detail: f.value("text_gaps", c)}}}
	if r.Cursor == "" {
		response.NextCursor = "gaps-two"
	}
	return response, nil
}
func (f invariantFixture) CorpusStatus(_ context.Context, _ serve.StatusRequest, c serve.Caller) (serve.StatusResponse, error) {
	return serve.StatusResponse{Versions: map[string]serve.VersionStatus{"fw:fw-alpha:1": {Sources: []serve.SourceStatus{{TextSource: f.value("source_files", c)}}}}}, nil
}
func (invariantFixture) CalibrationSearch(context.Context, string) (*float64, bool, error) {
	v := .5
	return &v, false, nil
}
func (invariantFixture) CalibrationRecipeHash() string { return "invented" }

func invariantObservations(t *testing.T, e invariantFixture, caller serve.Caller) []invariantObservation {
	t.Helper()
	observations, err := observe(context.Background(), e, EvaluationRequest{Cases: []GoldenCase{{ID: "case-a", Question: "invented query", Code: "fw-alpha", Target: "fw:fw-alpha:1#A", Adjudication: "accepted"}}}, caller)
	require.NoError(t, err)
	require.Len(t, observations, 22)
	return observations
}
func TestInvariant_DetectsInjectedLeakPerClass(t *testing.T) {
	before := invariantFixture{}
	reducedBefore := invariantObservations(t, before, freeCaller())
	fullBefore := invariantObservations(t, before, fullCaller())
	for _, class := range privateClasses() {
		t.Run(class, func(t *testing.T) {
			after := invariantFixture{changed: "yes", leak: class}
			r := compareInvariant("free-only", reducedBefore, invariantObservations(t, after, freeCaller()), fullBefore, invariantObservations(t, after, fullCaller()))
			require.False(t, r.Pass)
			require.False(t, r.Classes[class].Pass)
			require.True(t, r.Classes[class].PositiveControl)
			require.NotEmpty(t, r.Failures)
		})
	}
	after := invariantFixture{changed: "yes"}
	require.True(t, compareInvariant("free-only", reducedBefore, invariantObservations(t, after, freeCaller()), fullBefore, invariantObservations(t, after, fullCaller())).Pass)
}
func TestInvariant_RequiresEveryPositiveControl(t *testing.T) {
	before := invariantFixture{}
	a := invariantObservations(t, before, freeCaller())
	b := invariantObservations(t, before, fullCaller())
	for _, class := range privateClasses() {
		after := invariantFixture{changed: "yes", unchanged: class}
		r := compareInvariant("free-only", a, invariantObservations(t, after, freeCaller()), b, invariantObservations(t, after, fullCaller()))
		require.False(t, r.Pass)
		require.False(t, r.Classes[class].PositiveControl)
	}
}

func TestInvariant_UnverifiedQueryErrorsDoNotGate(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{{ID: "u", Question: "errored diagnostic", Adjudication: "unverified", Code: "fw-alpha", Target: "fw:fw-alpha:1#A"}}}
	_, err := observe(context.Background(), invariantFixture{}, req, freeCaller())
	require.NoError(t, err)
}

func TestInvariant_PaginationCannotFakePositiveControls(t *testing.T) {
	before := invariantObservations(t, invariantFixture{}, fullCaller())
	after := invariantObservations(t, invariantFixture{changed: "yes", unchanged: "source_files"}, fullCaller())
	for i, o := range after {
		if o.Operation == "read" && o.Page == 0 {
			after = append(after[:i], after[i+1:]...)
			break
		}
	}
	reduced := invariantObservations(t, invariantFixture{}, freeCaller())
	result := compareInvariant("free-only", reduced, reduced, before, after)
	require.False(t, result.Pass)
	require.False(t, result.Classes["source_files"].PositiveControl)
}

type replayLeakFixture struct {
	invariantFixture
	calls int
}

func (f *replayLeakFixture) Search(ctx context.Context, r serve.Request, caller serve.Caller) (serve.Response, error) {
	response, err := f.invariantFixture.Search(ctx, r, caller)
	f.calls++
	if f.calls == 2 {
		response.Message = "invented cache leak"
	}
	return response, err
}
func TestInvariant_DetectsCacheReplayDifference(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{{ID: "p", Question: "invented", Code: "fw-alpha", Adjudication: "accepted"}}}
	_, err := observe(context.Background(), &replayLeakFixture{}, req, freeCaller())
	require.Error(t, err)
}

func TestInvariant_AttributesLeakToMutatedClass(t *testing.T) {
	before := invariantObservations(t, invariantFixture{}, freeCaller())
	after := append([]invariantObservation(nil), before...)
	for i, o := range after {
		if o.Operation == "read" {
			after[i].Value = []byte(`{"private_evidence":"changed"}`)
			break
		}
	}
	fullBefore := invariantObservations(t, invariantFixture{}, fullCaller())
	fullAfter := invariantObservations(t, invariantFixture{changed: "yes"}, fullCaller())
	result := compareMutationClass("free-only", "mapping_evidence", before, after, fullBefore, fullAfter)
	require.False(t, result.Pass)
	for _, failure := range result.Failures {
		require.Equal(t, "mapping_evidence", failure.Class)
	}
	require.False(t, result.Classes["mapping_evidence"].Pass)
}

func TestInvariant_CoversRejectedAndOutOfReleaseQueries(t *testing.T) {
	req := EvaluationRequest{Cases: []GoldenCase{{ID: "r", Question: "invented rejected", Code: "fw-alpha", Adjudication: "rejected"}, {ID: "o", Question: "invented out of release", Code: "fw-alpha", Adjudication: "out_of_release"}}}
	observations, err := observe(context.Background(), invariantFixture{}, req, freeCaller())
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, o := range observations {
		if o.Operation == "search" {
			ids[o.CaseID] = true
		}
	}
	require.Equal(t, map[string]bool{"r": true, "o": true}, ids)
}
