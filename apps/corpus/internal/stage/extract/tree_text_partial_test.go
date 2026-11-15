package extract

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSelection_TreeTextPartial(t *testing.T) {
	tree := `{"success":true,"data":[{"id":"a","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi<br>Nội dung ngắn."}}]}`
	for _, tc := range []struct{ name, html, source, gap string }{
		{"accepted", "<p>Điều 1. Phạm vi</p><p>" + strings.Repeat("Nội dung dài. ", 60) + "</p>", "html_transcription", ""},
		{"rejected", "<p>Điều 2. Phạm vi</p><p>" + strings.Repeat("Nội dung dài. ", 60) + "</p>", "source_tree", "tree_text_partial"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := choose(context.Background(), transcriptionBundle(t, tree, tc.html), "vn:vbpl:1")
			if err != nil || got.source != tc.source || got.gap != tc.gap {
				t.Fatalf("selection=%+v err=%v", got, err)
			}
		})
	}
}

func TestPartialTreeSelection_Threshold(t *testing.T) {
	html := "<p>Điều 1.</p><p>" + strings.Repeat("x", 390) + "</p>"
	selected := selection{source: "html_transcription", data: []byte(html)}
	_, length, _, usable := partialTreeLengths([]provision{{NodeID: "a", Text: "x"}}, selected)
	if !usable || length%2 != 0 {
		t.Fatalf("length=%d usable=%v", length, usable)
	}
	for _, tc := range []struct {
		name string
		size int
		want string
	}{
		{"below half", length/2 - 1, "html_transcription"},
		{"exactly half", length / 2, "source_tree"},
		{"above half", length/2 + 1, "source_tree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []provision{{NodeID: "a", Text: strings.Repeat("x", tc.size)}}
			got, _, err := partialTreeSelection(rows, nil, selected, selection{source: "source_tree"})
			if err != nil || got.source != tc.want {
				t.Fatalf("source=%s err=%v", got.source, err)
			}
		})
	}
	got, _, err := partialTreeSelection([]provision{{NodeID: "a", Text: "x"}}, nil, selection{source: "none"}, selection{source: "source_tree"})
	if err != nil || got.source != "source_tree" || got.gap != "" {
		t.Fatalf("selection=%+v err=%v", got, err)
	}
}

func TestPartialTreeSelection_RecordLengths(t *testing.T) {
	selected := selection{source: "html_transcription", data: []byte("<p>Điều 1.</p><p>" + strings.Repeat("x", 400) + "</p>")}
	rows := []provision{{NodeID: "a", Text: "x"}}
	result, _, err := partialTreeSelection(rows, nil, selected, selection{source: "source_tree"})
	if err != nil || result.tree == nil {
		t.Fatalf("selection=%+v err=%v", result, err)
	}
	data, err := json.Marshal(textRecord{TextSource: result.source, Tree: result.tree})
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Tree struct {
			Rule               string `json:"rule"`
			TreeChars          *int   `json:"tree_chars"`
			TranscriptionChars *int   `json:"transcription_chars"`
		} `json:"tree"`
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Tree.Rule != "partial_tree" || record.Tree.TreeChars == nil || *record.Tree.TreeChars != 1 || record.Tree.TranscriptionChars == nil || *record.Tree.TranscriptionChars <= 2 {
		t.Fatalf("record=%s", data)
	}
}

func TestPartialTreeSelection_PreservesHeadingGap(t *testing.T) {
	selected := selection{source: "html_transcription", data: []byte("<p>Điều 2.</p><p>" + strings.Repeat("x", 400) + "</p>")}
	got, _, err := partialTreeSelection([]provision{{NodeID: "a", Text: "x"}}, nil, selected, selection{source: "source_tree", gap: "tree_headings_partial", reason: "rule 1 failed"})
	if err != nil || got.gap != "tree_headings_partial" || got.reason != "rule 1 failed" || got.extraGap != "tree_text_partial" {
		t.Fatalf("selection=%+v err=%v", got, err)
	}
}
