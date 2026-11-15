//go:build measurement

package extract

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

type measuredItem struct {
	Stage  string `json:"stage"`
	Key    string `json:"item_key"`
	State  string `json:"state"`
	Input  string `json:"input_hash"`
	Recipe string `json:"recipe_hash"`
}

func TestHeadingMeasurement_RecordedRelease(t *testing.T) {
	inputs, root := os.Getenv("HEADING_RELEASE_INPUTS"), os.Getenv("HEADING_ARTIFACTS")
	if inputs == "" || root == "" {
		t.Skip("set HEADING_RELEASE_INPUTS to vn/20261112_2 build inputs.json and HEADING_ARTIFACTS to its artifact root")
	}
	data, err := os.ReadFile(inputs)
	if err != nil {
		t.Fatal(err)
	}
	var inv struct {
		Items []measuredItem `json:"items"`
	}
	if err = json.Unmarshal(data, &inv); err != nil {
		t.Fatal(err)
	}
	fetches := make(map[string]string)
	for _, item := range inv.Items {
		if item.Stage == "fetch" && item.State == "done" {
			fetches[item.Key] = filepath.Join(root, "fetch", item.Input+"-"+item.Recipe)
		}
	}
	sourceTrees, candidates, switched, kept, characters, unchanged := 0, 0, 0, 0, 0, 0
	allHeadings, someHeadings := 0, 0
	reasons := make(map[int]int)
	for _, item := range inv.Items {
		if item.Stage != "extract" || item.State != "done" {
			continue
		}
		dir := filepath.Join(root, "extract", item.Input+"-"+item.Recipe)
		record, readErr := os.ReadFile(filepath.Join(dir, "text.json"))
		if readErr == nil {
			var meta struct {
				Source string `json:"text_source"`
			}
			if err = json.Unmarshal(record, &meta); err != nil {
				t.Fatal(err)
			}
			if meta.Source != "source_tree" {
				continue
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatal(readErr)
		}
		sourceTrees++
		data, err = os.ReadFile(filepath.Join(dir, "provisions.json"))
		if err != nil {
			t.Fatal(err)
		}
		var rows []provision
		if err = json.Unmarshal(data, &rows); err != nil {
			t.Fatal(err)
		}
		articles, headings := headingArticles(rows)
		if len(headings) == 0 {
			unchanged++
			continue
		}
		candidates++
		if articles == len(headings) {
			allHeadings++
		} else {
			someHeadings++
		}
		bundle := fetches[item.Key]
		doc, readErr := fetch.ReadDocument(bundle, item.Key)
		if readErr != nil {
			t.Fatal(readErr)
		}
		parts := strings.SplitN(item.Key, ":", 3)
		transcription, selectErr := selectFiles(bundle, parts[2], doc.Files, absentOCR{})
		if selectErr != nil {
			t.Fatal(selectErr)
		}
		comparison := compareTree(rows, transcription)
		if !comparison.SetAside {
			kept++
			reasons[comparison.FailedRule]++
			continue
		}
		switched++
		_, _, parsed := parseTranscription(transcription.data, transcription.name, transcription.hash)
		texts := make(map[string]string)
		for _, p := range parsed.Provisions {
			if p.Kind == "dieu" {
				texts[p.Anchor] = p.Text
			}
		}
		for _, row := range rows {
			if headings[row.NodeID] {
				characters += utf8.RuneCountInString(texts[row.Path]) - utf8.RuneCountInString(row.Text)
			}
		}
	}
	t.Logf("all_heading_documents=%d some_heading_documents=%d", allHeadings, someHeadings)
	t.Logf("source_tree=%d heading_candidates=%d switched=%d kept=%d first_failed_rule=%v net_article_characters_gained=%d trees_without_heading_only_articles=%d", sourceTrees, candidates, switched, kept, reasons, characters, unchanged)
	if sourceTrees != 2396 || candidates != 707 || allHeadings != 65 || someHeadings != 642 || switched != 338 || kept != 369 || characters != 1660734 || !reflect.DeepEqual(reasons, map[int]int{1: 197, 2: 97, 3: 26, 4: 49}) {
		t.Fatal("recorded release selection or net character counts changed")
	}
}
