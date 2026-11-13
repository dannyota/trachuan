package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/store"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestRows_RejectsUnverifiedArtifact(t *testing.T) {
	_, err := New(nil, recipe()).Rows(context.Background(), stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}, artifact.Result{Path: "missing"})
	if err == nil {
		t.Fatal("accepted unverified artifact")
	}
}

type cacheQuery []store.LookupEmbedRowsRow

func (q cacheQuery) LookupEmbedRows(_ context.Context, _ store.LookupEmbedRowsParams) ([]store.LookupEmbedRowsRow, error) {
	return q, nil
}

func TestCache_ReusePreservesArtifactBytes(t *testing.T) {
	ctx := context.Background()
	chunks := []chunk.Chunk{{Ordinal: 0, Content: "a"}, {Ordinal: 1, Content: "b"}, {Ordinal: 2, Content: "c"}}
	file := chunk.File{Version: 1, DocumentKey: "doc", Chunks: chunks}
	coverage := chunk.Coverage{}
	input := strings.Repeat("b", 64)
	baselineFake := &fake{}
	s := New(baselineFake.client(recipe()), recipe())
	idx, vectors, err := s.build(ctx, file, coverage, input)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifact.Store{Root: t.TempDir()}
	published, err := artifacts.Publish(ctx, "embed", input, s.Recipe(stage.Snapshot{}), func(dir string) error {
		data, e := json.Marshal(idx)
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, "index.json"), data, 0600); e != nil {
			return e
		}
		data, e = json.Marshal(makeReport(idx, coverage))
		if e != nil {
			return e
		}
		if e = os.WriteFile(filepath.Join(dir, "report.json"), data, 0600); e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dir, "vectors.f32"), vectors, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := cacheQuery{}
	for _, row := range idx.Rows {
		entries = append(entries, store.LookupEmbedRowsRow{InputSha256: row.InputSHA256, Row: int32(row.Row), TokenCount: int32(row.TokenCount), ArtifactID: 1, InputHash: input, Path: published.Path, Sha256: published.SHA256})
	}
	env := stage.Env{Artifacts: artifacts}
	for _, tc := range []struct {
		name string
		rows cacheQuery
		want []string
	}{
		{"all", entries, nil}, {"partial", entries[:2], []string{"\nc"}},
		{"mismatch", cacheQuery{func() store.LookupEmbedRowsRow { r := entries[0]; r.InputSha256 = entries[1].InputSha256; return r }()}, []string{"\na", "\nb", "\nc"}},
		{"out of range", cacheQuery{func() store.LookupEmbedRowsRow { r := entries[0]; r.Row = 100; return r }()}, []string{"\na", "\nb", "\nc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fake{}
			subject := New(f.client(recipe()), recipe())
			hits, e := subject.lookupFrom(ctx, tc.rows, env, stage.Run{}, stage.Item{}, file)
			if e != nil {
				t.Fatal(e)
			}
			got, data, e := subject.buildCached(ctx, file, input, hits)
			if e != nil {
				t.Fatal(e)
			}
			wantIndex, _ := json.Marshal(idx)
			gotIndex, _ := json.Marshal(got)
			wantReport, _ := json.Marshal(makeReport(idx, coverage))
			gotReport, _ := json.Marshal(makeReport(got, coverage))
			if !bytes.Equal(wantIndex, gotIndex) || !bytes.Equal(vectors, data) || !bytes.Equal(wantReport, gotReport) {
				t.Fatal("cached artifact differs")
			}
			if tc.want == nil {
				if len(f.calls) != 0 {
					t.Fatalf("calls=%v", f.calls)
				}
			} else if len(f.bodies) != 2 || !reflect.DeepEqual(f.bodies[0], tc.want) || !reflect.DeepEqual(f.bodies[1], tc.want) {
				t.Fatalf("bodies=%v", f.bodies)
			}
		})
	}
	if err = os.WriteFile(filepath.Join(artifacts.Abs(published.Path), "vectors.f32"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &fake{}
	subject := New(f.client(recipe()), recipe())
	hits, err := subject.lookupFrom(ctx, entries, env, stage.Run{}, stage.Item{}, file)
	if err != nil {
		t.Fatal(err)
	}
	got, data, err := subject.buildCached(ctx, file, input, hits)
	if err != nil {
		t.Fatal(err)
	}
	wantIndex, _ := json.Marshal(idx)
	gotIndex, _ := json.Marshal(got)
	if len(hits) != 0 || len(f.calls) != 2 || !bytes.Equal(wantIndex, gotIndex) || !bytes.Equal(vectors, data) {
		t.Fatal("invalid artifact did not re-embed")
	}
}
