//go:build integration

package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFetch_BaselineUsesLatestCommittedObservations(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := &fixtureServer{files: []map[string]any{entry("a", "a")}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	env.DB = db
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	recipe := s.Recipe(stage.Snapshot{})
	item := stage.Item{Key: "vn:vbpl:66518"}
	makeRun := func(day int) stage.Run {
		t.Helper()
		r := stage.Run{Country: "vn", Rel: fmt.Sprintf("209901%02d", day), Generation: 1, Snapshot: stage.Snapshot{Recipes: map[string]string{"fetch": recipe}}}
		queryErr := db.QueryRow(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn',$1,'running',1,'{}') RETURNING id`, r.Rel).Scan(&r.ID)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,generation,attempt) VALUES ($1,'fetch',$2,'pending',0,0,0)`, r.ID, item.Key); err != nil {
			t.Fatal(err)
		}
		return r
	}
	commit := func(r stage.Run, out stage.Output) {
		t.Helper()
		rows, rowsErr := s.Rows(ctx, env, *out.Artifact)
		if rowsErr != nil {
			t.Fatal(rowsErr)
		}
		ok, commitErr := stage.CommitItem(ctx, db, stage.Commit{RunID: r.ID, Stage: "fetch", ItemKey: item.Key, Generation: 1, Attempt: 1, RecipeHash: recipe, Artifact: out.Artifact, ArtifactRows: rows, Observations: out.Observations, Sources: []string{"vbpl"}, State: "done", SourceRead: true})
		if commitErr != nil || !ok {
			t.Fatalf("commit: %v %v", ok, commitErr)
		}
	}
	firstRun := makeRun(1)
	first, err := s.Process(ctx, env, firstRun, item)
	if err != nil {
		t.Fatal(err)
	}
	commit(firstRun, first)
	now = now.Add(time.Hour)
	secondRun := makeRun(2)
	noDB := env
	noDB.DB = nil
	second, err := s.Process(ctx, noDB, secondRun, item)
	if err != nil {
		t.Fatal(err)
	}
	if first.Artifact.SHA256 != second.Artifact.SHA256 {
		t.Fatal("bundles differ")
	}
	commit(secondRun, second)
	now = now.Add(time.Hour)
	thirdRun := makeRun(3)
	f.requests.Store(0)
	f.downloads.Store(0)
	third, err := s.Process(ctx, env, thirdRun, item)
	if err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 2 || f.downloads.Load() != 0 {
		t.Fatalf("calls %d downloads %d", f.requests.Load(), f.downloads.Load())
	}
	for _, o := range third.Observations {
		if o.Acquisition != "adopted" {
			continue
		}
		var ref []any
		if o.FetchedAt == nil || !o.FetchedAt.Equal(now.Add(-time.Hour)) || o.OriginRef == nil {
			t.Fatalf("origin: %+v", o)
		}
		if decodeErr := json.Unmarshal([]byte(*o.OriginRef), &ref); decodeErr != nil || ref[0] != float64(secondRun.ID) {
			t.Fatalf("ref %s", *o.OriginRef)
		}
	}
	commit(thirdRun, third)
	otherRun := makeRun(4)
	otherRun.Snapshot.Recipes["fetch"] = strings.Repeat("a", 64)
	f.requests.Store(0)
	if _, err = s.Process(ctx, env, otherRun, item); err != nil {
		t.Fatal(err)
	}
	if f.requests.Load() != 4 {
		t.Fatalf("other recipe calls %d", f.requests.Load())
	}
}
