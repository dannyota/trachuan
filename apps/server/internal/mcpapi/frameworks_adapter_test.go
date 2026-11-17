package mcpapi

import (
	"context"
	"reflect"
	"testing"
	"time"

	"danny.vn/trachuan/internal/frameworks/serve"
)

type frameworkAdapter struct {
	request any
	release string
	steps   []string
}

func (f *frameworkAdapter) ResolveFrameworks(_ context.Context, rel string) (FrameworkLease, error) {
	f.steps = append(f.steps, "resolve")
	f.release = rel
	return FrameworkLease{Engine: f, Done: func() { f.steps = append(f.steps, "done") }}, nil
}

func (*frameworkAdapter) FrameworkCodes() []string { return []string{"fw-alpha", "fw-beta"} }

func (f *frameworkAdapter) metadata(r any, caller serve.Caller) serve.Metadata {
	f.request = r
	f.steps = append(f.steps, "operation")
	return serve.Metadata{Release: "20990101", AccessSetVersion: "from-engine", AccessUnavailable: caller.AccessUnavailable}
}

func (f *frameworkAdapter) Search(_ context.Context, r serve.Request, c serve.Caller) (serve.Response, error) {
	m := f.metadata(r, c)
	return serve.Response{Release: m.Release, AccessSetVersion: m.AccessSetVersion, AccessUnavailable: m.AccessUnavailable}, nil
}

func (f *frameworkAdapter) Read(_ context.Context, r serve.ReadRequest, c serve.Caller) (serve.ReadResponse, error) {
	return serve.ReadResponse{Metadata: f.metadata(r, c), Items: []serve.ReadItemResult{{Complete: true}}}, nil
}

func (f *frameworkAdapter) CorpusStatus(_ context.Context, r serve.StatusRequest, c serve.Caller) (serve.StatusResponse, error) {
	return serve.StatusResponse{Metadata: f.metadata(r, c), RetrievalRecipeStatus: "placeholder"}, nil
}

func (f *frameworkAdapter) QualityGaps(_ context.Context, r serve.GapsRequest, c serve.Caller) (serve.GapsResponse, error) {
	return serve.GapsResponse{Metadata: f.metadata(r, c), RetrievalRecipeStatus: "placeholder"}, nil
}

func (f *frameworkAdapter) Mappings(_ context.Context, r serve.MappingsRequest, c serve.Caller) (serve.MappingsResponse, error) {
	return serve.MappingsResponse{Metadata: f.metadata(r, c)}, nil
}

func TestFrameworkAdapter_ForwardsInputsAndEngineMetadataWithoutCaching(t *testing.T) {
	f := &frameworkAdapter{}
	h := newHarness(t, func(c *Config) {
		c.Frameworks, c.OmitCompactRelated = true, true
		c.FrameworkResolver = f
		c.Caches = NewCaches(time.Now)
		c.FrameworkAccess = func(_ context.Context, id int64, codes []string) (serve.Caller, error) {
			if id != 7 || !reflect.DeepEqual(codes, []string{"fw-alpha", "fw-beta"}) {
				t.Fatal("incorrect access inputs")
			}
			f.steps = append(f.steps, "access")
			return serve.Caller{AccessUnavailable: true}, nil
		}
	})
	for _, tc := range []struct {
		name string
		args map[string]any
		want any
	}{
		{"corpus_status", map[string]any{}, serve.StatusRequest{Release: "20990101"}},
		{"search", map[string]any{"query": "invented", "frameworks": []string{"fw-alpha"}, "versions": []string{"fw:fw-alpha:1"}, "limit": 2, "cursor": "opaque"}, serve.Request{Query: "invented", Release: "20990101", Limit: 2, Cursor: "opaque", Filters: map[string][]string{"frameworks": {"fw-alpha"}, "versions": {"fw:fw-alpha:1"}}}},
		{"read", map[string]any{"items": []any{map[string]any{"handle": "fw:fw-alpha:1#A"}, map[string]any{"next": "opaque"}}}, serve.ReadRequest{Release: "20990101", Items: []serve.ReadItem{{Handle: "fw:fw-alpha:1#A"}, {Next: "opaque"}}}},
		{"quality_gaps", map[string]any{"versions": []string{"fw:fw-alpha:1"}, "kind": "text_incomplete", "cursor": "opaque"}, serve.GapsRequest{Release: "20990101", Versions: []string{"fw:fw-alpha:1"}, Kind: "text_incomplete", Cursor: "opaque"}},
		{"mappings", map[string]any{"from": "fw:fw-alpha:1", "to": "fw:fw-beta:1", "relation": []string{"related"}, "limit": 2, "cursor": "opaque"}, serve.MappingsRequest{Release: "20990101", From: "fw:fw-alpha:1", To: "fw:fw-beta:1", Relations: []string{"related"}, Limit: 2, Cursor: "opaque"}},
	} {
		tc.args["corpus"], tc.args["release"] = "frameworks", "20990101"
		for range 2 {
			f.steps = nil
			result := callTool(t, h, tc.name, tc.args)
			value := resultJSON(t, result)
			if result.IsError || value["release"] != "20990101" || value["access_set_version"] != "from-engine" || value["access_unavailable"] != true {
				t.Fatalf("incorrect engine metadata for %s: %v", tc.name, value)
			}
			if f.release != "20990101" || !reflect.DeepEqual(f.request, tc.want) || !reflect.DeepEqual(f.steps, []string{"resolve", "access", "operation", "done"}) {
				t.Fatalf("incorrect dispatch for %s: request=%+v steps=%v", tc.name, f.request, f.steps)
			}
		}
	}
}
