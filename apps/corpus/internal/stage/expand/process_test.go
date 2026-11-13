package expand

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/time/rate"
)

func TestExpand_ProcessRecordedFixtures(t *testing.T) {
	for _, id := range []string{"66518", "245a9a00-d447-11f1-84e4-cfbf13844ed1"} {
		t.Run(id, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				name := "detail-" + id + ".json"
				if strings.HasSuffix(r.URL.Path, "/diagram") {
					name = "diagram-" + id + ".json"
				}
				b, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", name))
				if err != nil {
					t.Error(err)
					return
				}
				_, _ = w.Write(b)
			}))
			defer server.Close()
			client := vbpl.NewWithBase(server.URL, "test", vbpl.WithTestHTTP(), vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(time.Millisecond, time.Millisecond, time.Millisecond))
			data, err := json.Marshal(source.List{Country: "vn"})
			if err != nil {
				t.Fatal(err)
			}
			env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
			out, err := New(client).Process(context.Background(), env, stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceList: data}}, stage.Item{Key: "vn:vbpl:" + id})
			if err != nil {
				t.Fatal(err)
			}
			if out.State != "done" || out.Artifact == nil || len(out.Sources) != 1 || out.Sources[0] != "vbpl" {
				t.Fatalf("output = %#v", out)
			}
			for _, name := range []string{"detail.json", "diagram.json", "observation.json"} {
				if _, statErr := os.Stat(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), name)); statErr != nil {
					t.Fatal(statErr)
				}
			}
			detail, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "detail-"+id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			diagram, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "diagram-"+id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			observation, err := Qualify("vn:vbpl:"+id, detail, diagram, nil)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(observation)
			if err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string][]byte{"detail.json": detail, "diagram.json": diagram, "observation.json": encoded} {
				got, readErr := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), name))
				if readErr != nil || !bytes.Equal(got, want) {
					t.Fatalf("%s differs from sequential output: %v", name, readErr)
				}
			}
		})
	}
}

func TestExpand_ProcessSourceFailuresAndMismatch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		kind   string
		retry  bool
	}{
		{name: "not found", status: http.StatusNotFound, kind: "not_found"},
		{name: "unavailable", status: http.StatusServiceUnavailable, kind: "source_unavailable"},
		{name: "rate limited", status: http.StatusTooManyRequests, retry: true},
		{name: "wrong detail id", status: http.StatusOK, body: `{"success":true,"data":{"id":"2"}}`, kind: "detail_id_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := vbpl.NewWithBase(server.URL, "test", vbpl.WithTestHTTP(), vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(time.Millisecond, time.Millisecond, time.Millisecond))
			out, err := New(client).Process(context.Background(), stage.Env{}, stage.Run{Country: "vn"}, stage.Item{Key: "vn:vbpl:1"})
			if len(out.Sources) != 1 || out.Sources[0] != "vbpl" {
				t.Fatalf("sources = %#v", out.Sources)
			}
			if tc.retry {
				var retry *stage.RetryableError
				if !errors.As(err, &retry) || retry.GapKind != "source_unavailable" || retry.Cause.Cause != stage.CauseHTTP429 {
					t.Fatalf("error = %#v", err)
				}
				return
			}
			if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != tc.kind {
				t.Fatalf("out=%#v err=%v", out, err)
			}
			if tc.kind == "source_unavailable" && (out.Gaps[0].Cause != stage.CauseHTTP5xx || out.Gaps[0].Origin != stage.OriginGateway || out.Gaps[0].HTTPStatus == nil || *out.Gaps[0].HTTPStatus != http.StatusServiceUnavailable) {
				t.Fatalf("gap cause = %#v", out.Gaps[0])
			}
		})
	}
}

func TestExpand_ProcessUsesRunCountry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/diagram") {
			_, _ = w.Write([]byte(`{"success":true,"data":{"documentNamesByType":{}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"1"}}`))
	}))
	defer server.Close()
	client := vbpl.NewWithBase(server.URL, "test", vbpl.WithTestHTTP(), vbpl.WithLimits(float64(rate.Inf), 8), vbpl.WithBackoffs(time.Millisecond, time.Millisecond, time.Millisecond))
	out, err := New(client).Process(context.Background(), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}, stage.Run{Country: "kh", Snapshot: stage.Snapshot{SourceList: []byte(`{"country":"kh"}`)}}, stage.Item{Key: "kh:vbpl:1"})
	if err != nil || out.State != "done" {
		t.Fatalf("output=%#v error=%v", out, err)
	}
}
