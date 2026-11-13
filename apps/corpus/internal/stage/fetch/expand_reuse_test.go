package fetch

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestFetch_ExpandObservationsOnBaselineReuse(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{}, bodies: map[string]string{}}
	s, env := setup(t, f)
	first := process(t, s, env)
	base := baselineFromOutput(t, env, first)
	if err := base.indexOrigins(context.Background()); err != nil {
		t.Fatal(err)
	}
	out, err := s.publishReuse(context.Background(), env, stage.Run{}, base, base.detail, []byte(`{"success":true,"data":{}}`), [3]time.Time{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, obs := range out.Observations {
		if obs.Path != "detail.json" && obs.Path != "diagram.json" {
			continue
		}
		if obs.Acquisition != "adopted" || obs.FetchedAt != nil || obs.OriginRef != nil {
			t.Errorf("observation = %+v", obs)
		}
	}
}

func TestFetch_ExpandSeedMatchesFreshBundle(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{}, bodies: map[string]string{}}
	s, env := setup(t, f)
	item := stage.Item{Key: "vn:vbpl:66518"}
	rel := "20990101_1"
	for controlSample(item.Key, rel) {
		rel += "1"
	}
	run := stage.Run{Country: "vn", Rel: rel}
	f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null,"viewCount":7}}`
	fresh, err := s.Process(context.Background(), env, run, item)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := make(map[string][]byte)
	for _, name := range []string{"detail.json", "diagram.json", "tree.json", "files.json", "report.json", "MANIFEST.sha256"} {
		wantFiles[name], err = os.ReadFile(filepath.Join(env.Artifacts.Abs(fresh.Artifact.Path), name))
		if err != nil {
			t.Fatal(err)
		}
	}
	diagram, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "diagram-66518.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.loadExpand = func(context.Context, stage.Env, stage.Run, stage.Item) ([]byte, []byte, bool, error) {
		return []byte(f.detailBody), diagram, true, nil
	}
	f.requests.Store(0)
	seeded, err := s.Process(context.Background(), env, run, item)
	if err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 2 {
		t.Fatalf("source requests=%d, want tree and files only", f.requests.Load())
	}
	if fresh.Artifact.SHA256 != seeded.Artifact.SHA256 || fresh.Message != seeded.Message {
		t.Fatalf("fresh=%+v seeded=%+v", fresh, seeded)
	}
	for name, want := range wantFiles {
		got, readErr := os.ReadFile(filepath.Join(env.Artifacts.Abs(seeded.Artifact.Path), name))
		if readErr != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s differs: %v", name, readErr)
		}
	}
	for _, obs := range seeded.Observations {
		if obs.Path == "detail.json" || obs.Path == "diagram.json" {
			if obs.Acquisition != "adopted" || obs.FetchedAt != nil || obs.OriginRef != nil {
				t.Errorf("observation = %+v", obs)
			}
		}
	}
}

func TestFetch_ExpandSeedKeepsBaselineDecisions(t *testing.T) {
	for _, tc := range []struct {
		name, date, reason string
		missing            bool
	}{
		{"reused", `"x"`, "reused", false},
		{"null", `null`, "null_date", false},
		{"changed", `"y"`, "date_changed", false},
		{"missing", `"x"`, "missing_attachment", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fixtureServer{files: []map[string]any{}, bodies: map[string]string{}, detailBody: `{"success":true,"data":{"id":"66518","updatedDate":"x","viewCount":2}}`}
			s, env := setup(t, f)
			base := baselineFromOutput(t, env, process(t, s, env))
			if tc.missing {
				base.report.Missing = []string{"missing.pdf"}
			}
			s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
				return base, false, nil
			}
			item := stage.Item{Key: "vn:vbpl:66518"}
			rel := "20990101_1"
			for controlSample(item.Key, rel) {
				rel += "1"
			}
			run := stage.Run{Country: "vn", Rel: rel}
			f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":` + tc.date + `,"viewCount":9}}`
			fresh, err := s.Process(context.Background(), env, run, item)
			if err != nil {
				t.Fatal(err)
			}
			diagram, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", "diagram-66518.json"))
			if err != nil {
				t.Fatal(err)
			}
			s.loadExpand = func(context.Context, stage.Env, stage.Run, stage.Item) ([]byte, []byte, bool, error) {
				return []byte(f.detailBody), diagram, true, nil
			}
			f.requests.Store(0)
			seeded, err := s.Process(context.Background(), env, run, item)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.Message != seeded.Message || !strings.Contains(seeded.Message, "read="+tc.reason) {
				t.Fatalf("fresh=%q seeded=%q", fresh.Message, seeded.Message)
			}
			if fresh.Artifact.SHA256 != seeded.Artifact.SHA256 {
				t.Fatal("bundle differs")
			}
			want := int32(2)
			if tc.reason == "reused" {
				want = 0
			}
			if f.requests.Load() != want {
				t.Fatalf("source calls=%d want=%d", f.requests.Load(), want)
			}
			for _, obs := range seeded.Observations {
				if (obs.Path == "detail.json" || obs.Path == "diagram.json") && (obs.Acquisition != "adopted" || obs.FetchedAt != nil || obs.OriginRef != nil) {
					t.Errorf("observation=%+v", obs)
				}
			}
		})
	}
}

func TestFetch_ControlBypassesExpandAndKeepsComparison(t *testing.T) {
	f := &fixtureServer{files: []map[string]any{}, bodies: map[string]string{}, detailBody: `{"success":true,"data":{"id":"66518","updatedDate":"x"}}`}
	s, env := setup(t, f)
	base := baselineFromOutput(t, env, process(t, s, env))
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		return base, false, nil
	}
	s.loadExpand = func(context.Context, stage.Env, stage.Run, stage.Item) ([]byte, []byte, bool, error) {
		t.Error("control loaded expand")
		return nil, nil, false, nil
	}
	item := stage.Item{Key: "vn:vbpl:66518"}
	rel := "20990101_1"
	for !controlSample(item.Key, rel) {
		rel += "1"
	}
	f.requests.Store(0)
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn", Rel: rel}, item)
	if err != nil || f.requests.Load() != 4 || !strings.HasPrefix(out.Message, "read=control outcome=same") {
		t.Fatalf("out=%+v calls=%d err=%v", out, f.requests.Load(), err)
	}
}

type expandQuery struct {
	row store.GetRunItemArtifactRow
	err error
	arg store.GetRunItemArtifactParams
}

func (q *expandQuery) GetRunItemArtifact(_ context.Context, arg store.GetRunItemArtifactParams) (store.GetRunItemArtifactRow, error) {
	q.arg = arg
	return q.row, q.err
}

func TestLoadExpand_OnlyDoneItemInSameRun(t *testing.T) {
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	recipe := strings.Repeat("a", 64)
	detail := []byte(`{"success":true,"data":{"id":"66518"}}`)
	diagram := []byte(`{"success":true,"data":{}}`)
	art, err := env.Artifacts.PublishContent(context.Background(), "expand", recipe, func(dir string) error {
		if err := os.WriteFile(filepath.Join(dir, "detail.json"), detail, 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "diagram.json"), diagram, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	run := stage.Run{ID: 42, Snapshot: stage.Snapshot{Recipes: map[string]string{"expand": recipe}}}
	item := stage.Item{Key: "vn:vbpl:66518"}
	for _, tc := range []struct {
		name, state string
		err         error
		want        bool
	}{
		{"done", "done", nil, true}, {"gap", "gap", nil, false}, {"nonseed", "", pgx.ErrNoRows, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &expandQuery{row: store.GetRunItemArtifactRow{State: tc.state, Path: art.Path, Sha256: art.SHA256}, err: tc.err}
			gotDetail, gotDiagram, found, err := loadExpandFrom(context.Background(), q, env, run, item)
			if err != nil || found != tc.want {
				t.Fatalf("found=%v err=%v", found, err)
			}
			if q.arg.RunID != run.ID || q.arg.Stage != "expand" || q.arg.ItemKey != item.Key {
				t.Fatalf("query=%+v", q.arg)
			}
			if found && (!bytes.Equal(gotDetail, detail) || !bytes.Equal(gotDiagram, diagram)) {
				t.Fatal("artifact bytes differ")
			}
		})
	}
}

func TestFetch_ExpandMissReadsSource(t *testing.T) {
	for _, name := range []string{"nonseed", "gap"} {
		t.Run(name, func(t *testing.T) {
			f := &fixtureServer{files: []map[string]any{}, bodies: map[string]string{}}
			s, env := setup(t, f)
			s.loadExpand = func(context.Context, stage.Env, stage.Run, stage.Item) ([]byte, []byte, bool, error) {
				return nil, nil, false, nil
			}
			item := stage.Item{Key: "vn:vbpl:66518"}
			rel := "20990101_1"
			for controlSample(item.Key, rel) {
				rel += "1"
			}
			out, err := s.Process(context.Background(), env, stage.Run{Country: "vn", Rel: rel}, item)
			if err != nil || out.State != "done" || f.requests.Load() != 4 {
				t.Fatalf("out=%+v calls=%d err=%v", out, f.requests.Load(), err)
			}
			for _, obs := range out.Observations {
				if (obs.Path == "detail.json" || obs.Path == "diagram.json") && (obs.Acquisition != "network" || obs.FetchedAt == nil) {
					t.Errorf("observation=%+v", obs)
				}
			}
		})
	}
}

func TestFetch_ExpandDetailMismatchIsGap(t *testing.T) {
	s, env := setup(t, &fixtureServer{})
	s.loadExpand = func(context.Context, stage.Env, stage.Run, stage.Item) ([]byte, []byte, bool, error) {
		return []byte(`{"success":true,"data":{"id":"other"}}`), []byte(`{}`), true, nil
	}
	item := stage.Item{Key: "vn:vbpl:66518"}
	rel := "20990101_1"
	for controlSample(item.Key, rel) {
		rel += "1"
	}
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn", Rel: rel}, item)
	if err != nil || out.State != "gap" || len(out.Gaps) != 1 || out.Gaps[0].Kind != "detail_id_mismatch" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
