package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func goldenInput(t *testing.T, name, value string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(os.Getenv("FRAMEWORKS_SOURCE_DIR"), "trachuan", name), []byte(value), 0600))
}

func goldenReview(hash string) string {
	return fmt.Sprintf("reviews:\n  - release: '20990101'\n    reviewer: coordinator\n    date: '2099-01-01'\n    csv_sha256: %s\n    added: []\n    rejected: []\n    retargeted: []\n    note: invented review\n", hash)
}

func runGoldenSnapshot(t *testing.T) {
	t.Helper()
	source, work, err := prepare()
	require.NoError(t, err)
	base := filepath.Join(work, "20990101")
	require.NoError(t, os.MkdirAll(base, 0700))
	require.NoError(t, runStep(context.Background(), base, "snapshot", DatabaseRequest{RunID: "invented"}, source, &bytes.Buffer{}, nil, nil))
}

func TestSnapshot_GoldenInputs(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint(present), func(t *testing.T) {
			work := fixture(t)
			if present {
				goldenInput(t, "golden.csv", "invented frozen input\n")
				goldenInput(t, "golden-review.yaml", goldenReview(digest([]byte("invented frozen input\n"))))
				goldenInput(t, "floors.yaml", "opaque private floors\n")
			}
			runGoldenSnapshot(t)
			dir := filepath.Join(work, "20990101/snapshot")
			m := readRows[manifest](t, filepath.Join(dir, "manifest.json"))[0]
			for _, name := range []string{"golden.csv", "golden-review.yaml", "floors.yaml"} {
				key := "trachuan/" + name
				value, ok := m.Inputs[key]
				require.True(t, ok, key)
				if present {
					require.NotNil(t, value)
					require.Equal(t, *value, m.Outputs[key])
				} else {
					require.Nil(t, value)
				}
			}
			require.NoError(t, os.RemoveAll(os.Getenv("FRAMEWORKS_SOURCE_DIR")))
			runGoldenSnapshot(t)
		})
	}
}

func TestSnapshot_RejectsUnreviewedAndMalformedReview(t *testing.T) {
	valid := goldenReview(digest([]byte("private invented question\n")))
	for name, review := range map[string]string{
		"missing": "", "wrong hash": goldenReview(strings.Repeat("0", 64)),
		"unknown field":  valid + "    unknown: value\n",
		"missing field":  strings.ReplaceAll(valid, "    added: []\n", ""),
		"invalid date":   strings.ReplaceAll(valid, "2099-01-01", "2099-02-30"),
		"reviewer":       strings.ReplaceAll(valid, "coordinator", "another"),
		"multiline note": strings.ReplaceAll(valid, "note: invented review", "note: |\n      first\n      second"),
		"extra document": valid + "---\nreviews: []\n",
	} {
		t.Run(name, func(t *testing.T) {
			work := fixture(t)
			goldenInput(t, "golden.csv", "private invented question\n")
			if review != "" {
				goldenInput(t, "golden-review.yaml", review)
			}
			var out bytes.Buffer
			err := testRun(context.Background(), "20990101", &out)
			require.ErrorContains(t, err, "snapshot")
			require.NotContains(t, err.Error()+out.String(), "private invented question")
			require.NoDirExists(t, filepath.Join(work, "20990101/snapshot"))
		})
	}
}
