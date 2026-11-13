//go:build integration

package run

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStatus_WithdrawnUsesLatestVerifiedPair(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	ids := map[string]int64{}
	snapshot, _ := json.Marshal(stage.Snapshot{})
	for _, rel := range []string{"20990101_1", "20990101_2", "20990101_3", "20990101_4", "20990102_1"} {
		var id int64
		if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES('vn',$1,'paused',$2) RETURNING id`, rel, snapshot).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[rel] = id
	}
	key := "vn:vbpl:pin"
	number := "71/2099/QH15"
	before := membership.Entry{DocumentKey: key, Source: "vbpl", SourceID: "pin", Disposition: "active", Class: "primary", RawNumber: &number, Supports: []membership.SupportTuple{{RuleKind: "named", RuleValue: number}}}
	after := before
	after.Disposition = "withdrawn"
	q := store.New(db)
	if err = membership.PublishPhase(ctx, q, ids["20990101_1"], membership.Final, []membership.Entry{before}); err != nil {
		t.Fatal(err)
	}
	if err = q.EnsureRunScope(ctx, ids["20990102_1"]); err != nil {
		t.Fatal(err)
	}
	phase := "final"
	prior, err := q.GetRunScope(ctx, ids["20990101_1"])
	if err != nil {
		t.Fatal(err)
	}
	pred := ids["20990101_1"]
	if _, err = q.SetMembershipPredecessor(ctx, store.SetMembershipPredecessorParams{RunID: ids["20990102_1"], PredecessorRunID: &pred, PredecessorPhase: &phase, PredecessorSnapshotHash: prior.FinalHash}); err != nil {
		t.Fatal(err)
	}
	if err = membership.PublishPhase(ctx, q, ids["20990102_1"], membership.Seeds, []membership.Entry{after}); err != nil {
		t.Fatal(err)
	}
	wanted := map[string]string{}
	for _, rel := range []string{"20990101_1", "20990101_2", "20990101_3", "20990101_4"} {
		for _, name := range []string{"expand", "fetch"} {
			if (rel == "20990101_3" && name == "fetch") || (rel == "20990101_4" && name == "expand") {
				continue
			}
			art, publishErr := artifacts.PublishContent(ctx, name, stage.RecipeHash(name, rel), func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "detail.json"), []byte(`{"data":{"id":"pin","docNum":"71/2099/QH15"}}`), 0600)
			})
			if publishErr != nil {
				t.Fatal(publishErr)
			}
			var artifactID int64
			if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, name, key, art.InputHash, stage.RecipeHash(name, rel), art.Path, art.SHA256).Scan(&artifactID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES($1,$2,$3,'done',0,$4)`, ids[rel], name, key, artifactID); err != nil {
				t.Fatal(err)
			}
			switch rel {
			case "20990101_2":
				if err = os.WriteFile(filepath.Join(artifacts.Abs(art.Path), "detail.json"), []byte("altered"), 0600); err != nil {
					t.Fatal(err)
				}
			case "20990101_1":
				wanted[name] = art.SHA256
			}
		}
	}
	row, err := q.GetRun(ctx, store.GetRunParams{Country: "vn", Rel: "20990102_1"})
	if err != nil {
		t.Fatal(err)
	}
	report := Report{Run: row}
	if err = report.loadPinStatus(ctx, q, artifacts); err != nil {
		t.Fatal(err)
	}
	if len(report.NewlyWithdrawnNamed) != 1 {
		t.Fatalf("withdrawn=%+v", report.NewlyWithdrawnNamed)
	}
	doc := report.NewlyWithdrawnNamed[0]
	if doc.Number != number || doc.Rel != "20990101_1" || doc.Expand != wanted["expand"] || doc.Fetch != wanted["fetch"] || doc.Message != "" {
		t.Fatalf("withdrawn=%+v", doc)
	}
	data, err := json.Marshal(report)
	if err != nil || !strings.HasPrefix(string(data), `{"newly_withdrawn_named":`) {
		t.Fatalf("report order=%s err=%v", data, err)
	}
}
