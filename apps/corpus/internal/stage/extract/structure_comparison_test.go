package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type clauseDifference struct {
	Document, Article string
	Tree, Parsed      []string
}

func TestExtract_RecordedTranscriptionClauseComparison(t *testing.T) {
	differences := []clauseDifference{}
	for _, id := range []string{"145780", "27845", "67878"} {
		dir := filepath.Join("testdata", "structure")
		data, err := os.ReadFile(filepath.Join(dir, id+"-tree.json"))
		if err != nil {
			t.Fatal(err)
		}
		tree, err := vbpl.DecodeTree(context.Background(), bytes.NewReader(data), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
		if err != nil {
			t.Fatal(err)
		}
		original, _, err := extractTree(context.Background(), tree)
		if err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile(filepath.Join(dir, id+"-content.html"))
		if err != nil {
			t.Fatal(err)
		}
		parsed, _, _ := parseTranscription(data, id+"_content.html", "recorded")
		treeClauses, parsedClauses := map[string][]string{}, map[string][]string{}
		articles := map[string]string{}
		for _, p := range original {
			if p.Kind == "dieu" {
				articles[p.NodeID] = p.Path
				treeClauses[p.Path] = []string{}
			}
		}
		for _, p := range original {
			if p.Kind == "khoan" && p.ParentNodeID != nil {
				if article, ok := articles[*p.ParentNodeID]; ok {
					treeClauses[article] = append(treeClauses[article], p.Path)
				}
			}
		}
		articles = map[string]string{}
		for _, p := range parsed {
			if p.Kind == "dieu" {
				articles[p.NodeID] = p.Path
			}
		}
		for _, p := range parsed {
			if p.Kind == "khoan" && p.ParentNodeID != nil {
				if article, ok := articles[*p.ParentNodeID]; ok {
					parsedClauses[article] = append(parsedClauses[article], p.Path)
				}
			}
		}
		for article := range parsedClauses {
			if _, ok := treeClauses[article]; !ok {
				treeClauses[article] = []string{}
			}
		}
		clauseCount := 0
		for _, clauses := range treeClauses {
			clauseCount += len(clauses)
		}
		wantCount := map[string]int{"145780": 12, "27845": 40, "67878": 34}[id]
		if clauseCount != wantCount {
			t.Fatalf("%s tree clauses=%d want %d", id, clauseCount, wantCount)
		}
		t.Logf("%s: %d articles, %d clauses", id, len(treeClauses), clauseCount)
		var anchors []string
		for article := range treeClauses {
			anchors = append(anchors, article)
		}
		sort.Strings(anchors)
		for _, article := range anchors {
			got := parsedClauses[article]
			if got == nil {
				got = []string{}
			}
			if !reflect.DeepEqual(treeClauses[article], got) {
				differences = append(differences, clauseDifference{id, article, treeClauses[article], got})
			}
		}
	}
	data, err := json.MarshalIndent(differences, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join("testdata", "structure", "differences.json"))
	if err != nil {
		t.Fatalf("comparison differences:\n%s", data)
	}
	var want []clauseDifference
	if err = json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(differences, want) {
		t.Fatalf("comparison differences:\n%s", data)
	}
	t.Logf("comparison differences:\n%s", data)
}
