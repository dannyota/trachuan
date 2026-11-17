package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type fakeDatabase struct{ failure error }

func (d fakeDatabase) Build(_ context.Context, req DatabaseRequest, _ func(context.Context, *pgx.Conn) error, publish func() error) error {
	if d.failure != nil {
		return d.failure
	}
	if err := os.WriteFile(req.DumpPath, []byte("synthetic dump"), 0600); err != nil {
		return err
	}
	return publish()
}

type observingDatabase struct {
	fakeDatabase
	request DatabaseRequest
}

func (d *observingDatabase) Build(ctx context.Context, req DatabaseRequest, load func(context.Context, *pgx.Conn) error, publish func() error) error {
	d.request = req
	return d.fakeDatabase.Build(ctx, req, load, publish)
}

func TestBuild_OwnershipUsesRequestedReleaseAndRealWorkRoot(t *testing.T) {
	work := fixture(t)
	target := filepath.Join(work, "nested/20990102")
	require.NoError(t, os.MkdirAll(target, 0700))
	require.NoError(t, os.Symlink(target, filepath.Join(work, "20990101")))
	database := &observingDatabase{}
	require.NoError(t, Run(context.Background(), "20990101", &bytes.Buffer{}, fakeEmbedder{}, database))
	realWork, err := filepath.EvalSymlinks(work)
	require.NoError(t, err)
	require.Equal(t, realWork, database.request.WorkDir)
	require.Equal(t, "20990101", database.request.Rel)
	require.FileExists(t, filepath.Join(target, "build/frameworks.dump"))
}

func TestBuild_ManifestAndResume(t *testing.T) {
	work := fixture(t)
	var out bytes.Buffer
	require.NoError(t, testRun(context.Background(), "20990101", &out))
	base := filepath.Join(work, "20990101")
	rows := readRows[buildManifest](t, filepath.Join(base, "build/build.json"))
	require.Len(t, rows, 1)
	require.Equal(t, int64(14), rows[0].Dump.Size)
	require.Equal(t, digest([]byte("synthetic dump")), rows[0].Dump.SHA256)
	require.Equal(t, "build", rows[0].Recipe.Name)
	require.FileExists(t, filepath.Join(base, "evaluate/report.json"))
	inputs, err := stepInputs(base, "build")
	require.NoError(t, err)
	require.Len(t, inputs, 5)
	require.NoError(t, Run(context.Background(), "20990101", &out, fakeEmbedder{}, fakeDatabase{failure: errors.New("must skip database")}))
	require.Contains(t, out.String(), "build skipped")
	require.NoError(t, os.WriteFile(filepath.Join(base, "build/frameworks.dump"), []byte("corrupt"), 0600))
	require.ErrorContains(t, testRun(context.Background(), "20990101", &out), "build: output validation failed")
}

func TestBuild_RejectsHashConsistentInvalidSidecar(t *testing.T) {
	work := fixture(t)
	require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
	dir := filepath.Join(work, "20990101/build")
	rows := readRows[buildManifest](t, filepath.Join(dir, "build.json"))
	rows[0].Dump.Size++
	data, err := json.Marshal(rows[0])
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "build.json"), data, 0600))
	m := readRows[manifest](t, filepath.Join(dir, "manifest.json"))[0]
	m.Outputs, err = outputHashes(dir)
	require.NoError(t, err)
	data, err = json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600))
	require.ErrorContains(t, testRun(context.Background(), "20990101", &bytes.Buffer{}), "build manifest mismatch")
}

func TestDatabaseInputs_RejectsUnknownAndDuplicateGaps(t *testing.T) {
	for _, kind := range []string{"unknown", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			work := fixture(t)
			require.NoError(t, testRun(context.Background(), "20990101", &bytes.Buffer{}))
			base := filepath.Join(work, "20990101")
			rows := readRows[Gap](t, filepath.Join(base, "parse/gaps.jsonl"))
			if kind == "unknown" {
				rows[0].Kind = "unknown"
			} else {
				rows = append(rows, rows[0])
			}
			var data []byte
			for _, row := range rows {
				encoded, err := json.Marshal(row)
				require.NoError(t, err)
				data = append(data, append(encoded, '\n')...)
			}
			require.NoError(t, os.WriteFile(filepath.Join(base, "parse/gaps.jsonl"), data, 0600))
			_, err := readDatabaseInputs(base)
			require.Error(t, err)
		})
	}
}

func TestBuild_FailureIsPrivateAndRetryable(t *testing.T) {
	work := fixture(t)
	var out bytes.Buffer
	err := Run(context.Background(), "20990101", &out, fakeEmbedder{}, fakeDatabase{failure: errors.New("synthetic licensed SQL value")})
	require.ErrorContains(t, err, "build")
	require.NotContains(t, err.Error()+out.String(), "licensed SQL value")
	require.NoDirExists(t, filepath.Join(work, "20990101/build"))
	require.NoError(t, testRun(context.Background(), "20990101", &out))
}

func TestGapPolicy_ClassAndBlocking(t *testing.T) {
	for _, tc := range []struct {
		kind, class    string
		free, licensed bool
	}{
		{"mapping_unresolved", "public", false, false}, {"mapping_dropped", "public", false, false},
		{"title_missing", "public", false, true}, {"ocr_blocked", "text", true, true},
		{"ocr_flagged", "text", false, false}, {"text_incomplete", "text", false, false},
		{"chunk_omission", "text", false, false}, {"embedding_input_too_long", "text", false, false},
		{"embedding_input_bytes_exceeded", "text", false, false},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			for access, want := range map[string]bool{"free": tc.free, "licensed": tc.licensed} {
				class, blocks, err := classifyGap(tc.kind, access)
				require.NoError(t, err)
				require.Equal(t, tc.class, class)
				require.Equal(t, want, blocks)
			}
		})
	}
	_, _, err := classifyGap("unknown", "licensed")
	require.Error(t, err)
	_, _, err = classifyGap("title_missing", "")
	require.Error(t, err)
}

func (d fakeDatabase) Evaluate(_ context.Context, _ EvaluationRequest, publish func(EvaluationResult) error) error {
	return publish(EvaluationResult{Restored: true, RetrievalRecipeStatus: "placeholder", Cohorts: map[string]CohortMetrics{}, Cases: []CaseResult{}, Gaps: []GapTotal{}})
}
