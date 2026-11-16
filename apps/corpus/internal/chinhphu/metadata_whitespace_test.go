package chinhphu

import (
	"testing"
)

func TestDetail_TrimMetadataOccurrences(t *testing.T) {
	d, err := New().ParseDetail(fixture(t, "metadata-whitespace.html"), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"142/2026/NĐ-CP", "2026-04-30", "2026-05-01", "2030-05-01"}
	for i, value := range want {
		if d.Occurrences[i].Value != value {
			t.Errorf("occurrence %d = %q, want %q", i, d.Occurrences[i].Value, value)
		}
	}
}

func TestSearch_TrimMetadata(t *testing.T) {
	node, err := parseHTML([]byte(`<div id="document_page_info">1 - 1 | 1</div><table id="ctrl_191017_163_grvDocument"><tr><td><a href="?docid=218029">link</a><span class="code">
                    142/2026/NĐ-CP </span><span class="issued-date">
                    30-04-2026 </span></td></tr></table>`))
	if err != nil {
		t.Fatal(err)
	}
	items, _, _, _, ok := parseSearch(node)
	if !ok || len(items) != 1 || items[0].RawNumber == nil || *items[0].RawNumber != "142/2026/NĐ-CP" || items[0].IssueDate == nil || *items[0].IssueDate != "2026-04-30" {
		t.Fatalf("items = %+v, valid = %v", items, ok)
	}
}

func TestMetadata_InvalidDatesRemainTrimmed(t *testing.T) {
	for _, raw := range []string{"31-02-2026", "30/04/2026x"} {
		t.Run(raw, func(t *testing.T) {
			node, err := parseHTML([]byte(`<div id="document_page_info">1 - 1 | 1</div><table id="ctrl_191017_163_grvDocument"><tr><td><a href="?docid=1">link</a><span class="issued-date">
   ` + raw + ` </span></td></tr></table>`))
			if err != nil {
				t.Fatal(err)
			}
			items, _, _, _, ok := parseSearch(node)
			if !ok || len(items) != 1 || items[0].IssueDate == nil || *items[0].IssueDate != raw {
				t.Fatalf("items = %+v", items)
			}
			d, err := New().ParseDetail([]byte(`<div id="ctrl_190596_91_Content"><table><tr><td>Ngày ban hành</td><td>
   `+raw+` </td></tr></table></div>`), "")
			if err != nil || len(d.Occurrences) != 1 || d.Occurrences[0].Value != raw {
				t.Fatalf("detail = %+v, error = %v", d, err)
			}
		})
	}
}
