package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestUnavailable_UsesBaseline(t *testing.T) {
	f := &fixtureServer{}
	s, env := setup(t, f)
	first := process(t, s, env)
	b := baselineFromOutput(t, env, first)
	b.row.Path, b.row.Sha256 = first.Artifact.Path, first.Artifact.SHA256
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
	f.detailStatus = 500
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil || out.State != "done" || out.Artifact == nil || out.Artifact.SHA256 != first.Artifact.SHA256 || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" || parseReadMessage(out.Message).reason != "baseline_after_unavailable" {
		t.Fatalf("output=%+v err=%v", out, err)
	}
	if len(out.Observations) != len(b.observations) {
		t.Fatalf("observations=%+v", out.Observations)
	}
	for _, o := range out.Observations {
		if o.Acquisition != "adopted" || o.OriginRef == nil || o.FetchedAt == nil {
			t.Fatalf("observation=%+v", o)
		}
	}
}

func TestUnavailable_NoValidBaseline(t *testing.T) {
	for _, mode := range []string{"none", "invalid", "other gap"} {
		t.Run(mode, func(t *testing.T) {
			f := &fixtureServer{}
			s, env := setup(t, f)
			first := process(t, s, env)
			b := baselineFromOutput(t, env, first)
			b.row.Path, b.row.Sha256 = first.Artifact.Path, first.Artifact.SHA256
			s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
				if mode == "none" {
					return nil, false, nil
				}
				return b, mode == "invalid", nil
			}
			if mode == "other gap" {
				f.missing = true
			} else {
				f.detailStatus = 500
			}
			out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
			want := "source_unavailable"
			if mode == "other gap" {
				want = "not_found"
			}
			if err != nil || out.State != "gap" || out.Artifact != nil || len(out.Gaps) != 1 || out.Gaps[0].Kind != want {
				t.Fatalf("output=%+v err=%v", out, err)
			}
		})
	}
}

func TestUnavailable_FallbackAtEachGateway(t *testing.T) {
	for _, endpoint := range []string{"detail", "diagram", "tree", "files"} {
		t.Run(endpoint, func(t *testing.T) {
			f := &fixtureServer{}
			s, env := setup(t, f)
			first := process(t, s, env)
			b := baselineFromOutput(t, env, first)
			b.row.Path, b.row.Sha256 = first.Artifact.Path, first.Artifact.SHA256
			s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
			f.detailBody = `{"success":true,"data":{"id":"66518"}}`
			switch endpoint {
			case "detail":
				f.detailStatus = 500
			case "diagram":
				f.diagramStatus = 500
			case "tree":
				f.treeStatus = 500
			case "files":
				f.filesStatus = 500
			}
			out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
			if err != nil || out.State != "done" || out.Artifact == nil || out.Artifact.SHA256 != first.Artifact.SHA256 || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" {
				t.Fatalf("output=%+v err=%v", out, err)
			}
		})
	}
}

func TestUnavailable_HTTP429RemainsRetryable(t *testing.T) {
	f := &fixtureServer{}
	s, env := setup(t, f)
	first := process(t, s, env)
	b := baselineFromOutput(t, env, first)
	b.row.Path, b.row.Sha256 = first.Artifact.Path, first.Artifact.SHA256
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
	f.detailStatus = 429
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
	var retry *stage.RetryableError
	if !errors.As(err, &retry) || retry.Cause.Cause != stage.CauseHTTP429 || out.Artifact != nil || len(out.Gaps) != 0 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestUnavailable_Deferred429IsTerminal(t *testing.T) {
	s, env := setup(t, &fixtureServer{})
	first := process(t, s, env)
	b := baselineFromOutput(t, env, first)
	b.row.Path, b.row.Sha256 = first.Artifact.Path, first.Artifact.SHA256
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) { return b, false, nil }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "601")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	s.client = testClient(server.URL)
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil || out.State != "done" || out.Artifact == nil || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
