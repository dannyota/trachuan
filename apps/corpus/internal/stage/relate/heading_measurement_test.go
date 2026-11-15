//go:build measurement

package relate

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHeadingRelationsMeasurement_Inventory(t *testing.T) {
	ctx := context.Background()
	cc, rel, out := os.Getenv("MEASURE_COUNTRY"), os.Getenv("MEASURE_REL"), os.Getenv("MEASURE_OUT")
	dbURL, root := os.Getenv("CORPUS_DATABASE_URL"), os.Getenv("CORPUS_ARTIFACTS")
	if cc == "" || rel == "" || out == "" || dbURL == "" || root == "" {
		t.Fatal("set MEASURE_COUNTRY, MEASURE_REL, MEASURE_OUT, CORPUS_DATABASE_URL and CORPUS_ARTIFACTS")
	}
	config, setupErr := pgxpool.ParseConfig(dbURL)
	if setupErr != nil {
		t.Fatal("invalid database configuration")
	}
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	db, setupErr := pgxpool.NewWithConfig(ctx, config)
	if setupErr != nil {
		t.Fatal("open database")
	}
	defer db.Close()
	q := store.New(db)
	stored, setupErr := q.GetRun(ctx, store.GetRunParams{Country: cc, Rel: rel})
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	var snapshot stage.Snapshot
	if setupErr = json.Unmarshal(stored.Snapshot, &snapshot); setupErr != nil {
		t.Fatal(setupErr)
	}
	list, setupErr := source.Parse(snapshot.SourceList)
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	artifacts := artifact.Store{Root: root}
	extracts, setupErr := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: stored.ID, Stage: "extract"})
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	fetches, setupErr := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: stored.ID, Stage: "fetch"})
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	fetchByKey := make(map[string]store.ListStageItemArtifactsRow, len(fetches))
	for _, item := range fetches {
		if item.State == "done" {
			fetchByKey[item.ItemKey] = item
		}
	}
	file, setupErr := os.Create(out)
	if setupErr != nil {
		t.Fatal(setupErr)
	}
	t.Cleanup(func() { _ = file.Close() })
	w := csv.NewWriter(file)
	w.Comma = '\t'
	if setupErr = w.Write([]string{"source_key", "source_anchor", "kind", "target_number", "target_anchor"}); setupErr != nil {
		t.Fatal(setupErr)
	}
	counts := map[string]int{}
	documents, anchored := 0, 0
	for _, item := range extracts {
		if item.State != "done" || !strings.HasPrefix(item.ItemKey, "vn:vbpl:") {
			continue
		}
		input, ok := fetchByKey[item.ItemKey]
		if !ok {
			continue
		}
		if item.Path == nil || item.Sha256 == nil || input.Path == nil || input.Sha256 == nil {
			t.Fatalf("missing artifact for %s", item.ItemKey)
		}
		dir := artifacts.Abs(*item.Path)
		if err := verifyFetch(dir, *item.Sha256); err != nil {
			t.Fatal(err)
		}
		var gaps []stage.Gap
		if err := readJSON(filepath.Join(dir, "report.json"), &gaps); err != nil {
			t.Fatal(err)
		}
		heading := false
		for _, gap := range gaps {
			heading = heading || gap.Kind == "tree_headings_only" || gap.Kind == "tree_headings_partial"
		}
		if !heading {
			continue
		}
		bundle := artifacts.Abs(*input.Path)
		if err := verifyFetch(bundle, *input.Sha256); err != nil {
			t.Fatal(err)
		}
		doc, err := fetch.ReadDocument(bundle, item.ItemKey)
		if err != nil {
			t.Fatal(err)
		}
		if doc.IsConsolidated {
			continue
		}
		detail, err := os.ReadFile(filepath.Join(bundle, "detail.json"))
		if err != nil {
			t.Fatal(err)
		}
		diagram, err := os.ReadFile(filepath.Join(bundle, "diagram.json"))
		if err != nil {
			t.Fatal(err)
		}
		links, _, err := relations(detail, diagram, list.RelationCodes)
		if err != nil {
			t.Fatal(err)
		}
		rows, report, err := textRelations(dir, links, list.CentralIssuers)
		if err != nil {
			t.Fatal(err)
		}
		if report.Gaps["relation_text_source_incomplete"] != 0 {
			continue
		}
		documents++
		var nodes []chunk.Node
		if err = readJSON(filepath.Join(dir, "provisions.json"), &nodes); err != nil {
			t.Fatal(err)
		}
		paths := map[string]string{}
		for _, node := range nodes {
			paths[node.NodeID] = node.Path
		}
		for _, row := range rows[len(links):] {
			if row.SourceNodeID == nil || paths[*row.SourceNodeID] == "" {
				t.Fatalf("missing source anchor for %s", item.ItemKey)
			}
			if err = w.Write([]string{item.ItemKey, paths[*row.SourceNodeID], row.Kind, row.TargetNumber, row.TargetAnchor}); err != nil {
				t.Fatal(err)
			}
			counts[row.Kind]++
			if row.TargetAnchor != "" {
				anchored++
			}
		}
	}
	w.Flush()
	if writeErr := w.Error(); writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	t.Logf("documents newly read: %d; relations naming target anchor: %d", documents, anchored)
	for _, kind := range kinds {
		t.Logf("%s: %d", kind, counts[kind])
	}
}
