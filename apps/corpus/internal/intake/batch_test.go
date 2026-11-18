package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func groupID(key, anchor, kind, target string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{key, anchor, kind, target}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

const batchManifest = `{"batch_id":"BATCH","created_at":"2099-11-18T00:00:00Z","report_ids":[1,2,3,4,5,6,7]}` + "\n"

func writeBatch(t *testing.T, overrides map[string]string) string {
	t.Helper()
	files := map[string]string{
		"manifest.json":          strings.Replace(batchManifest, "BATCH", "b2099", 1),
		"corrections-draft.yaml": "- key: \"vn:vbpl:7\"\n  number: \"1/2099/ND-CP\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n",
		"validity-review.jsonl":  `{"document_key":"vn:vbpl:1","anchor":"dieu-5","kind":"wrong_validity","target":"","reports":[{"report_id":1,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:1","anchor":"dieu-5","document_number":"1/2099/ND-CP","release":"20991118_1","kind":"wrong_validity","target":"","suggestion":{}}]}` + "\n",
		"text-issues.jsonl":      `{"document_key":"vn:vbpl:2","anchor":"","kind":"wrong_text","target":"","reports":[{"report_id":2,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:2","anchor":"","document_number":"2/2099/ND-CP","release":"20991118_1","kind":"wrong_text","target":"","suggestion":{}},{"report_id":3,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:2","anchor":"","document_number":"2/2099/ND-CP","release":"20991118_1","kind":"wrong_text","target":"","suggestion":{}}]}` + "\n",
		"citation-review.jsonl": `{"document_key":"vn:vbpl:3","anchor":"dieu-2","kind":"broken_citation","target":"5/2099/ND-CP","reports":[{"report_id":4,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:3","anchor":"dieu-2","document_number":"3/2099/ND-CP","release":"20991118_1","kind":"broken_citation","target":"5/2099/ND-CP","suggestion":{"document_number":"5/2099/ND-CP"}}]}` + "\n" +
			`{"document_key":"","anchor":"","kind":"missing_document","target":"9/2099/ND-CP","reports":[{"report_id":5,"created_at":"2099-11-18T00:00:00Z","document_key":"","anchor":"","document_number":"","release":"20991118_1","kind":"missing_document","target":"9/2099/ND-CP","suggestion":{"document_number":"9/2099/ND-CP"}},{"report_id":6,"created_at":"2099-11-18T00:00:00Z","document_key":"","anchor":"","document_number":"","release":"20991118_1","kind":"missing_document","target":"9/2099/ND-CP","suggestion":{"document_number":"9/2099/ND-CP"}}]}` + "\n",
	}
	for name, content := range overrides {
		if content == "-" {
			delete(files, name)
			continue
		}
		files[name] = content
	}
	dir := filepath.Join(t.TempDir(), "b2099")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadBatch_ConvertsEveryGroup(t *testing.T) {
	records, err := ReadBatch("vn", writeBatch(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 5 {
		t.Fatalf("records = %+v", records)
	}
	want := map[string]Record{
		groupID("vn:vbpl:7", "", "wrong_validity", ""):                    {Kind: "wrong_validity", Target: Target{Key: "vn:vbpl:7"}, Claim: Claim{Code: "status_conflict"}},
		groupID("vn:vbpl:1", "dieu-5", "wrong_validity", ""):              {Kind: "wrong_validity", Target: Target{Key: "vn:vbpl:1", Anchor: "dieu-5"}, Claim: Claim{Code: "status_conflict"}},
		groupID("vn:vbpl:2", "", "wrong_text", ""):                        {Kind: "wrong_text", Target: Target{Key: "vn:vbpl:2"}, Claim: Claim{Code: "reported"}},
		groupID("vn:vbpl:3", "dieu-2", "broken_citation", "5/2099/ND-CP"): {Kind: "broken_citation", Target: Target{Key: "vn:vbpl:3", Anchor: "dieu-2"}, Claim: Claim{Code: "wrong_number"}},
		groupID("", "", "missing_document", "9/2099/ND-CP"):               {Kind: "missing_document", Target: Target{Number: "9/2099/ND-CP"}, Claim: Claim{Code: "missing_document"}},
	}
	for i, r := range records {
		w, ok := want[r.OriginID]
		if !ok || r.Origin != "issue_report" || r.Kind != w.Kind || r.Target != w.Target || r.Claim != w.Claim {
			t.Fatalf("record %d = %+v", i, r)
		}
		if i > 0 && records[i-1].OriginID >= r.OriginID {
			t.Fatal("records are not ordered")
		}
	}
	if _, err = Parse("vn", Encode(records)); err != nil {
		t.Fatalf("converted records do not parse: %v", err)
	}
}

func TestReadBatch_ReplayIsIdempotent(t *testing.T) {
	dir := writeBatch(t, nil)
	first, _ := ReadBatch("vn", dir)
	second, _ := ReadBatch("vn", dir)
	merged, added, _, err := Merge("vn", File{Records: first}, second)
	if err != nil || len(added) != 0 || len(merged) != len(first) {
		t.Fatalf("added=%d err=%v", len(added), err)
	}
}

func TestReadBatch_RefusesFailedVerification(t *testing.T) {
	report := func(id string) string {
		return `{"report_id":` + id + `,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:2","anchor":"","document_number":"","release":"r","kind":"wrong_text","target":"","suggestion":{}}`
	}
	group := func(reports ...string) string {
		return `{"document_key":"vn:vbpl:2","anchor":"","kind":"wrong_text","target":"","reports":[` + strings.Join(reports, ",") + `]}` + "\n"
	}
	cases := map[string]map[string]string{
		"batch id differs from the directory": {"manifest.json": strings.Replace(batchManifest, "BATCH", "other", 1)},
		"manifest missing":                    {"manifest.json": "-"},
		"manifest unknown field":              {"manifest.json": `{"batch_id":"b2099","created_at":"2099-11-18T00:00:00Z","report_ids":[1],"account":"x"}` + "\n"},
		"manifest duplicate id":               {"manifest.json": strings.Replace(strings.Replace(batchManifest, "BATCH", "b2099", 1), "[1,2", "[1,1,2", 1)},
		"report not in manifest":              {"text-issues.jsonl": group(report("2"), report("99"))},
		"report repeated":                     {"text-issues.jsonl": group(report("2"), report("2"))},
		"manifest id left over":               {"corrections-draft.yaml": "[]\n"},
		"draft with no manifest id left":      {"manifest.json": `{"batch_id":"b2099","created_at":"2099-11-18T00:00:00Z","report_ids":[1,2,3,4,5,6,7]}` + "\n", "corrections-draft.yaml": "- key: \"vn:vbpl:7\"\n  number: \"1/2099/ND-CP\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n- key: \"vn:vbpl:8\"\n  number: \"1/2099/ND-CP\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n"},
		"report differs from its group":       {"text-issues.jsonl": strings.Replace(group(report("2"), report("3")), `"anchor":"","kind":"wrong_text","target":"","reports"`, `"anchor":"dieu-1","kind":"wrong_text","target":"","reports"`, 1)},
		"kind in the wrong file":              {"validity-review.jsonl": group(report("1"))},
		"account field":                       {"text-issues.jsonl": strings.Replace(group(report("2"), report("3")), `"suggestion":{}`, `"suggestion":{},"account_id":5`, 1)},
		"unknown draft field":                 {"corrections-draft.yaml": "- key: \"vn:vbpl:7\"\n  number: \"1\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n  email: \"a@b.c\"\n"},
		"extra file":                          {"notes.txt": "hello"},
		"missing file":                        {"text-issues.jsonl": "-"},
		"invalid key":                         {"text-issues.jsonl": strings.ReplaceAll(group(report("2"), report("3")), "vn:vbpl:2", "us:x")},
		"trailing json after manifest":        {"manifest.json": strings.Replace(batchManifest, "BATCH", "b2099", 1) + `{}` + "\n"},
		"second yaml document in draft":       {"corrections-draft.yaml": "- key: \"vn:vbpl:7\"\n  number: \"1\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n---\n[]\n"},
		"duplicate draft":                     {"manifest.json": `{"batch_id":"b2099","created_at":"2099-11-18T00:00:00Z","report_ids":[1,2,3,4,5,6,7,8]}` + "\n", "corrections-draft.yaml": strings.Repeat("- key: \"vn:vbpl:7\"\n  number: \"1\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n", 2)},
		"draft key not a key":                 {"corrections-draft.yaml": "- key: \"vn:someone@mail.com\"\n  number: \"1\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n"},
		"duplicate group":                     {"text-issues.jsonl": group(report("2")) + group(report("3"))},
	}
	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadBatch("vn", writeBatch(t, overrides)); err == nil {
				t.Fatal("batch accepted")
			}
		})
	}
}

func TestReadBatch_RefusesSymlinkAndFile(t *testing.T) {
	dir := writeBatch(t, nil)
	if err := os.Remove(filepath.Join(dir, "text-issues.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "manifest.json"), filepath.Join(dir, "text-issues.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBatch("vn", dir); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err := ReadBatch("vn", filepath.Join(dir, "manifest.json")); err == nil {
		t.Fatal("file accepted as directory")
	}
}

func TestReadBatch_RefusesOversizeFile(t *testing.T) {
	dir := writeBatch(t, nil)
	if err := os.Truncate(filepath.Join(dir, "text-issues.jsonl"), maxBatchFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBatch("vn", dir); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadBatch_AmbiguousAnchorFallsBackToTheDocument(t *testing.T) {
	report := func(id, anchor string) string {
		return `{"report_id":` + id + `,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:2","anchor":"` + anchor + `","document_number":"","release":"r","kind":"wrong_text","target":"","suggestion":{}}`
	}
	group := func(id, anchor string) string {
		return `{"document_key":"vn:vbpl:2","anchor":"` + anchor + `","kind":"wrong_text","target":"","reports":[` + report(id, anchor) + `]}` + "\n"
	}
	dir := writeBatch(t, map[string]string{
		"manifest.json":          `{"batch_id":"b2099","created_at":"2099-11-18T00:00:00Z","report_ids":[1,2,3,4,7]}` + "\n",
		"citation-review.jsonl":  "",
		"validity-review.jsonl":  `{"document_key":"vn:vbpl:1","anchor":"dieu-5","kind":"wrong_validity","target":"","reports":[{"report_id":1,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:1","anchor":"dieu-5","document_number":"","release":"r","kind":"wrong_validity","target":"","suggestion":{}}]}` + "\n",
		"text-issues.jsonl":      group("2", "dieu-5~2") + group("3", "dieu-"+strings.Repeat("9", 70)) + group("4", "dieu-5/diem-đ"),
		"corrections-draft.yaml": "- key: \"vn:vbpl:7\"\n  number: \"1\"\n  set: { status_class: \"expired\" }\n  reason: \"TODO\"\n  evidence: { quote: \"TODO\" }\n  reviewed: \"\"\n",
	})
	records, err := ReadBatch("vn", dir)
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]int{}
	for _, r := range records {
		if r.Kind == "wrong_text" {
			anchors[r.Target.Anchor]++
		}
	}
	if anchors[""] != 2 || anchors["dieu-5/diem-đ"] != 1 {
		t.Fatalf("anchors = %v", anchors)
	}
	for _, r := range records {
		if r.Target.Key == "vn:vbpl:2" && r.OriginID == GroupID("vn:vbpl:2", "dieu-5~2", "wrong_text", "") {
			return
		}
	}
	t.Fatal("the origin id must still name the exact group")
}

func TestReadBatch_RefusedGroupIsNamed(t *testing.T) {
	line := `{"document_key":"","anchor":"","kind":"missing_document","target":"NO DIGIT","reports":[{"report_id":5,"created_at":"2099-11-18T00:00:00Z","document_key":"","anchor":"","document_number":"","release":"r","kind":"missing_document","target":"NO DIGIT","suggestion":{}}]}` + "\n"
	dir := writeBatch(t, map[string]string{"citation-review.jsonl": line})
	_, err := ReadBatch("vn", dir)
	if err == nil || !strings.Contains(err.Error(), "group "+GroupID("", "", "missing_document", "NO DIGIT")[:12]) {
		t.Fatalf("err = %v", err)
	}
}

func TestReadBatch_ReadsTheServerExportFixture(t *testing.T) {
	records, err := ReadBatch("vn", filepath.Join("testdata", "batch", "b2099"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 8 {
		t.Fatalf("records = %d: %+v", len(records), records)
	}
	byID := map[string]Record{}
	for _, r := range records {
		byID[r.OriginID] = r
	}
	if r := byID[GroupID("vn:vbpl:2", "dieu-5~2", "wrong_text", "")]; r.Target != (Target{Key: "vn:vbpl:2"}) {
		t.Fatalf("ambiguous anchor record = %+v", r)
	}
	if r := byID[GroupID("vn:vbpl:2", "dieu-5/diem-đ", "wrong_text", "")]; r.Target.Anchor != "dieu-5/diem-đ" {
		t.Fatalf("anchor record = %+v", r)
	}
	if r := byID[GroupID("vn:vbpl:1", "", "wrong_validity", "")]; r.Claim.Code != "status_conflict" || r.Target.Key != "vn:vbpl:1" {
		t.Fatalf("draft record = %+v", r)
	}
	if r := byID[GroupID("vn:vbpl:3", "dieu-2", "missing_document", "12/2099/QÐ-UBND & 3")]; r.Target.Number != "12/2099/QÐ-UBND & 3" {
		t.Fatalf("number record = %+v", r)
	}
	if _, err = Parse("vn", Encode(records)); err != nil {
		t.Fatal(err)
	}
}
