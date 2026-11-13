package pinned

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

type pinReader struct {
	rows  map[string]store.GetPinnedArtifactRow
	calls int
}

func (q *pinReader) GetPinnedArtifact(_ context.Context, p store.GetPinnedArtifactParams) (store.GetPinnedArtifactRow, error) {
	q.calls++
	if p.Country != "vn" || p.Rel != "20990101_1" || p.ItemKey != "vn:vbpl:pin" {
		return store.GetPinnedArtifactRow{}, errors.New("wrong producer")
	}
	row, ok := q.rows[p.Stage]
	if !ok || row.Sha256 != p.Sha256 || row.Stage != p.Stage {
		return store.GetPinnedArtifactRow{}, errors.New("wrong artifact")
	}
	return row, nil
}

func pinFixture(t *testing.T) (artifact.Store, source.Pin, *pinReader) {
	t.Helper()
	st := artifact.Store{Root: t.TempDir()}
	pin := source.Pin{Key: "vn:vbpl:pin", Number: "71/2099/QH15", Rel: "20990101_1", Since: "2099-01-02"}
	q := &pinReader{rows: map[string]store.GetPinnedArtifactRow{}}
	for _, name := range []string{"expand", "fetch"} {
		a, err := st.PublishContent(context.Background(), name, strings.Repeat("a", 64), func(dir string) error {
			files := map[string]string{"detail.json": `{"data":{"id":"pin","docNum":"71/2099/QH15"}}`, "diagram.json": `{"data":[]}`}
			if name == "expand" {
				files["observation.json"] = `{}`
			} else {
				files["tree.json"] = `{"data":[]}`
				files["files.json"] = `[]`
				files["report.json"] = `{"missing":[],"too_large":[]}`
			}
			for n, v := range files {
				if err := os.WriteFile(filepath.Join(dir, n), []byte(v), 0600); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		q.rows[name] = store.GetPinnedArtifactRow{RunID: 1, Generation: 1, Attempt: 1, Stage: name, Path: a.Path, Sha256: a.SHA256, InputHash: a.InputHash, RecipeHash: strings.Repeat("a", 64)}
		if name == "expand" {
			pin.Expand = a.SHA256
		} else {
			pin.Fetch = a.SHA256
		}
	}
	return st, pin, q
}

func TestVerify_PinValidation(t *testing.T) {
	for _, name := range []string{"valid", "missing artifact", "altered file", "wrong rel", "wrong stage", "wrong key", "number mismatch", "duplicate key", "duplicate normalized number"} {
		t.Run(name, func(t *testing.T) {
			st, pin, q := pinFixture(t)
			pins := []source.Pin{pin}
			switch name {
			case "missing artifact":
				if err := os.RemoveAll(st.Abs(q.rows["fetch"].Path)); err != nil {
					t.Fatal(err)
				}
			case "altered file":
				if err := os.WriteFile(filepath.Join(st.Abs(q.rows["fetch"].Path), "tree.json"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong rel":
				pins[0].Rel = "20990101_2"
			case "wrong stage":
				row := q.rows["expand"]
				row.Stage = "fetch"
				q.rows["expand"] = row
			case "wrong key":
				pins[0].Key = "vn:vbpl:other"
			case "number mismatch":
				pins[0].Number = "72/2099/QH15"
			case "duplicate key":
				pins = append(pins, pin)
			case "duplicate normalized number":
				other := pin
				other.Key = "vn:vbpl:other"
				other.Number = " 71 / 2099 / qh15 "
				pins = append(pins, other)
			}
			err := Verify(context.Background(), q, st, "vn", pins)
			if (err == nil) != (name == "valid") {
				t.Fatalf("Verify error=%v", err)
			}
			if name == "duplicate normalized number" && q.calls != 0 {
				t.Fatalf("duplicate set read artifacts first: %d", q.calls)
			}
		})
	}
}
