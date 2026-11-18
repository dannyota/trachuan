package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/intake"
)

const importGroup = `{"document_key":"vn:vbpl:2","anchor":"","kind":"wrong_text","target":"","reports":[{"report_id":2,"created_at":"2099-11-18T00:00:00Z","document_key":"vn:vbpl:2","anchor":"","document_number":"2/2099/ND-CP","release":"r","kind":"wrong_text","target":"","suggestion":{}}]}` + "\n"

func importFixture(t *testing.T, reports string) string {
	t.Helper()
	root := t.TempDir()
	batch := filepath.Join(root, "b2099")
	if err := os.Mkdir(batch, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"manifest.json":          `{"batch_id":"b2099","created_at":"2099-11-18T00:00:00Z","report_ids":[2]}` + "\n",
		"corrections-draft.yaml": "[]\n",
		"validity-review.jsonl":  "",
		"text-issues.jsonl":      importGroup,
		"citation-review.jsonl":  "",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(batch, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "sources"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "vn-reports.yaml"), []byte(reports), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return batch
}

func execImport(args ...string) (int, string, string) {
	var out, errout bytes.Buffer
	code := Run(context.Background(), append([]string{"findings", "import"}, args...), &out, &errout)
	return code, out.String(), errout.String()
}

func TestFindingsImport_PrintsNewRecordsAndWritesNothing(t *testing.T) {
	batch := importFixture(t, "[]\n")
	code, out, errout := execImport("vn", batch)
	if code != 0 || !strings.Contains(out, `origin: "issue_report"`) || !strings.Contains(errout, "1 added") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
	if data, _ := os.ReadFile("sources/vn-reports.yaml"); string(data) != "[]\n" {
		t.Fatalf("file changed: %q", data)
	}
	if _, err := intake.Parse("vn", []byte(out)); err != nil {
		t.Fatalf("printed records do not parse: %v", err)
	}
}

func TestFindingsImport_WriteIsIdempotent(t *testing.T) {
	batch := importFixture(t, "[]\n")
	if code, _, errout := execImport("vn", batch, "--write"); code != 0 {
		t.Fatalf("code=%d err=%q", code, errout)
	}
	first, _ := os.ReadFile("sources/vn-reports.yaml")
	if file, err := intake.Parse("vn", first); err != nil || file.Len() != 1 {
		t.Fatalf("file = %q, %v", first, err)
	}
	code, out, errout := execImport("vn", batch, "--write")
	second, _ := os.ReadFile("sources/vn-reports.yaml")
	if code != 0 || string(first) != string(second) || !strings.Contains(out+errout, "0 added") {
		t.Fatalf("replay changed the file: code=%d out=%q err=%q", code, out, errout)
	}
	code, out, _ = execImport("vn", batch)
	if code != 0 || out != "" {
		t.Fatalf("replay printed records: %q", out)
	}
	entries, _ := os.ReadDir("sources")
	if len(entries) != 1 {
		t.Fatalf("left files behind: %v", entries)
	}
}

func TestFindingsImport_RefusesBadInput(t *testing.T) {
	batch := importFixture(t, "[]\n")
	if err := os.WriteFile(filepath.Join(batch, "text-issues.jsonl"), []byte(strings.Replace(importGroup, `"report_id":2`, `"report_id":9`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errout := execImport("vn", batch, "--write"); code != 1 || errout == "" {
		t.Fatalf("failed batch accepted: code=%d", code)
	}
	if data, _ := os.ReadFile("sources/vn-reports.yaml"); string(data) != "[]\n" {
		t.Fatal("a refused batch changed the file")
	}
	for _, args := range [][]string{{"vn"}, {"vn", batch, "--other"}, {"zz9", batch}} {
		if code, _, _ := execImport(args...); code != 2 {
			t.Fatalf("%v: code=%d", args, code)
		}
	}
}

func TestFindingsImport_AddsTheFirstBoundAndReplayContinues(t *testing.T) {
	batch := importFixture(t, "[]\n")
	var lines strings.Builder
	var ids []string
	for i := 1; i <= intake.MaxNewPerImport+2; i++ {
		key := fmt.Sprintf("vn:vbpl:%d", i)
		fmt.Fprintf(&lines, `{"document_key":%q,"anchor":"","kind":"wrong_text","target":"","reports":[{"report_id":%d,"created_at":"2099-11-18T00:00:00Z","document_key":%q,"anchor":"","document_number":"","release":"r","kind":"wrong_text","target":"","suggestion":{}}]}`+"\n", key, i, key)
		ids = append(ids, fmt.Sprint(i))
	}
	manifest := `{"batch_id":"b2099","created_at":"2099-11-18T00:00:00Z","report_ids":[` + strings.Join(ids, ",") + `]}` + "\n"
	for name, content := range map[string]string{"text-issues.jsonl": lines.String(), "manifest.json": manifest} {
		if err := os.WriteFile(filepath.Join(batch, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errout := execImport("vn", batch, "--write")
	if code != 0 || !strings.Contains(out, "500 added, 2 remaining") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
	code, out, _ = execImport("vn", batch, "--write")
	if code != 0 || !strings.Contains(out, "2 added, 0 remaining") {
		t.Fatalf("replay: code=%d out=%q", code, out)
	}
	if file, _, err := intake.ParseFile("vn", "sources/vn-reports.yaml"); err != nil || file.Len() != intake.MaxNewPerImport+2 {
		t.Fatalf("len=%d err=%v", file.Len(), err)
	}
}

func TestFindingsImport_RefusesAMergeTheNextParseWouldRefuse(t *testing.T) {
	key := "vn:vbpl:" + strings.Repeat("a", 56)
	anchor := "dieu-" + strings.Repeat("1", 59)
	record := func(i int) intake.Record {
		return intake.Record{Origin: "eval", OriginID: fmt.Sprintf("case-%059d", i), Kind: "wrong_validity", Target: intake.Target{Key: key, Anchor: anchor}, Claim: intake.Claim{Code: "repealed_by", Acting: intake.Identity{Key: key, Anchor: anchor}}}
	}
	size := len(intake.Encode([]intake.Record{record(0)}))
	count := (intake.MaxFileBytes - 200) / size
	if count >= intake.MaxRecords {
		t.Fatalf("a record of %d bytes cannot reach the size limit", size)
	}
	records := make([]intake.Record, count)
	for i := range records {
		records[i] = record(i)
	}
	for width := 59; width > 0; width-- {
		filler := record(count)
		filler.Target.Anchor = "dieu-" + strings.Repeat("1", width)
		if total := len(intake.Encode(append(records, filler))); total <= intake.MaxFileBytes-60 {
			records = append(records, filler)
			break
		}
	}
	existing := intake.Encode(records)
	if _, err := intake.Parse("vn", existing); err != nil {
		t.Fatal(err)
	}
	batch := importFixture(t, string(existing))
	code, out, errout := execImport("vn", batch, "--write")
	if code != 1 || !strings.Contains(errout, "nothing written") || out != "" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errout)
	}
	if after, _ := os.ReadFile("sources/vn-reports.yaml"); string(after) != string(existing) {
		t.Fatal("a refused merge changed the file")
	}
	if code, out, _ = execImport("vn", batch); code != 1 || out != "" {
		t.Fatalf("print mode printed records the next parse would refuse: code=%d", code)
	}
}
