package fetch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"

	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

func TestSourceRule_ExhaustedCallsBecomeGaps(t *testing.T) {
	for _, cause := range []string{"timeout", "connection_error", "http_5xx", "non_json_200", "unknown"} {
		t.Run(cause, func(t *testing.T) {
			out, err := apiError(&vbpl.Failure{Origin: "transport", Cause: cause, Endpoint: "doc/detail"}, "1", true)
			if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" {
				t.Fatalf("VBPL: %+v %v", out, err)
			}
			out, err = chinhphuError(&chinhphu.Failure{Origin: "transport", Cause: cause, Endpoint: "page"}, "1", true)
			if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" {
				t.Fatalf("chinhphu: %+v %v", out, err)
			}
		})
	}
}

func TestSourceRule_MissingFileKeepsOtherFilesAndAdoptionGap(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{entry("a", "a"), entry("b", "b")}, bodies: map[string]string{"a": "one", "b": "two"}, downloadStatuses: map[string]int{"a": 500}}
	s, env := setup(t, f)
	out := process(t, s, env)
	want := stage.Gap{Kind: "missing_attachment", Detail: "a"}
	for _, reader := range []*Stage{s, New(nil)} {
		res, err := reader.Resolve(context.Background(), env, stage.Run{}, stage.Item{Key: "vn:vbpl:66518"}, *out.Artifact)
		if err != nil || len(res.Gaps) != 1 || res.Gaps[0] != want {
			t.Fatalf("resolution=%+v error=%v", res, err)
		}
	}
	if f.downloads.Load() != 3 || parseReadMessage(out.Message).counts["failure_http_500"] != 1 {
		t.Fatalf("downloads=%d message=%s", f.downloads.Load(), out.Message)
	}
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "files.json"))
	if err != nil || !strings.Contains(string(data), `"name":"b"`) || strings.Contains(string(data), `"name":"a"`) {
		t.Fatalf("files=%s error=%v", data, err)
	}
}

func TestSourceRule_NonDetail404IsUnavailable(t *testing.T) {
	for _, f := range []*fixtureServer{{treeStatus: 404}, {diagramStatus: 404}, {filesStatus: 404}} {
		s, env := setup(t, f)
		out, err := s.Process(context.Background(), env, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:66518"})
		if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "source_unavailable" {
			t.Fatalf("output=%+v error=%v", out, err)
		}
	}
}
