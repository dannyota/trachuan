package extract

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
)

func TestSelection_InvalidTreeIdentityReturnsSafely(t *testing.T) {
	if os.Getenv("HEADING_IDENTITY_CHILD") == "1" {
		debug.SetMaxStack(1024 * 1024)
		tree := `{"success":true,"data":[{"id":"shared","level":"Article","title":"Điều 1. Phạm vi","content":{"content":"Điều 1. Phạm vi"},"children":[{"id":"shared","level":"Clause","title":"Khoản 1","content":{"content":"1. Nội dung."}}]},{"id":"heading","level":"Article","title":"Điều 2. Quy định","content":{"content":"Điều 2. Quy định"}}]}`
		html := "<p>Điều 1. Phạm vi</p><p>1. Nội dung.</p><p>Điều 2. Quy định</p><p>" + strings.Repeat("Quy định chung. ", 30) + "</p>"
		selected, _, err := choose(context.Background(), transcriptionBundle(t, tree, html), "vn:vbpl:1")
		if err != nil || selected.source != "source_tree" || selected.gap != "tree_headings_partial" || !strings.Contains(selected.reason, "rule 3 failed") {
			t.Fatalf("selection=%+v err=%v", selected, err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSelection_InvalidTreeIdentityReturnsSafely$")
	cmd.Env = append(os.Environ(), "HEADING_IDENTITY_CHILD=1", "GORACE="+os.Getenv("GORACE")+" atexit_sleep_ms=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		first, _, _ := strings.Cut(string(output), "\n")
		t.Fatalf("selection subprocess failed: %v: %s", err, first)
	}
}

func TestSelection_DuplicateRootIdentityKeepsTree(t *testing.T) {
	f := recordedHeadingFixture(t)
	var tree map[string]any
	if err := json.Unmarshal(f.Tree, &tree); err != nil {
		t.Fatal(err)
	}
	nodes := tree["data"].([]any)
	duplicate := map[string]any{"id": nodes[1].(map[string]any)["id"], "level": "Other", "title": "", "content": map[string]string{"content": ""}}
	tree["data"] = append(nodes, duplicate)
	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	selected, _, err := choose(context.Background(), transcriptionBundle(t, string(data), f.HTML), "vn:vbpl:1")
	if err != nil || selected.source != "source_tree" || selected.gap != "tree_headings_partial" || !strings.Contains(selected.reason, "rule 3 failed") {
		t.Fatalf("source=%s gap=%s detail=%s err=%v", selected.source, selected.gap, selected.reason, err)
	}
}

func TestCompareTree_InvalidIdentityFailsBeforeRendering(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []provision
	}{
		{"empty id", []provision{{Kind: "dieu", Path: "dieu-1"}}},
		{"missing parent", []provision{{NodeID: "article", Kind: "dieu", Path: "dieu-1", ParentNodeID: identityParent("missing")}}},
		{"duplicate root", []provision{{NodeID: "article", Kind: "dieu", Path: "dieu-1"}, {NodeID: "article", Kind: "dieu", Path: "dieu-2"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comparison := compareTree(tc.rows, selection{})
			if comparison.FailedRule != 3 || comparison.SetAside {
				t.Fatalf("comparison=%+v", comparison)
			}
		})
	}
}

func identityParent(id string) *string { return &id }
