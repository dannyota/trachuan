package intake

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validRecords = `- origin: "eval"
  origin_id: "case-2099-1"
  kind: "missing_document"
  target: { number: "14/2099/ND-CP" }
  claim: { code: "missing_document" }
- origin: "issue_report"
  origin_id: "` + "0000000000000000000000000000000000000000000000000000000000000001" + `"
  kind: "wrong_validity"
  target: { key: "vn:vbpl:1", anchor: "dieu-5" }
  claim: { code: "repealed_by", acting: { key: "vn:vbpl:2", anchor: "dieu-30/khoan-2" } }
- origin: "sc365"
  origin_id: "sc-1"
  kind: "wrong_text"
  target: { key: "vn:vbpl:3", anchor: "" }
  claim: { code: "reported" }
`

func TestParse_AcceptsEveryShape(t *testing.T) {
	file, err := Parse("vn", []byte(validRecords))
	if err != nil {
		t.Fatal(err)
	}
	if file.Len() != 3 {
		t.Fatalf("records = %d", file.Len())
	}
	r := file.Records[1]
	if r.Target.Key != "vn:vbpl:1" || r.Target.Anchor != "dieu-5" || r.Claim.Acting.Key != "vn:vbpl:2" || r.Claim.Acting.Anchor != "dieu-30/khoan-2" {
		t.Fatalf("record = %+v", r)
	}
	if file.Records[0].Target.Number != "14/2099/ND-CP" {
		t.Fatalf("record = %+v", file.Records[0])
	}
}

func TestParse_EmptyMeansNoRecords(t *testing.T) {
	for _, data := range []string{"", "[]\n"} {
		file, err := Parse("vn", []byte(data))
		if err != nil || file.Len() != 0 {
			t.Fatalf("%q: len=%d err=%v", data, file.Len(), err)
		}
	}
}

func TestParse_RefusesInvalid(t *testing.T) {
	good := strings.Split(validRecords, "- origin: \"issue_report\"")[0]
	cases := map[string]string{
		"unknown field":         strings.Replace(good, `claim:`, "note: \"x\"\n  claim:", 1),
		"unknown origin":        strings.Replace(good, `"eval"`, `"email"`, 1),
		"unknown kind":          strings.Replace(good, `"missing_document"`+"\n  target", `"other"`+"\n  target", 1),
		"unknown code":          strings.Replace(good, `code: "missing_document"`, `code: "other"`, 1),
		"free text origin id":   strings.Replace(good, `case-2099-1`, `a question here`, 1),
		"number not normalized": strings.Replace(good, `14/2099/ND-CP`, `14/2099/nd-cp`, 1),
		"number with account":   strings.Replace(good, `14/2099/ND-CP`, `a@b.c`, 1),
		"number target on text": strings.Replace(good, `kind: "missing_document"`, `kind: "wrong_text"`, 1),
		"key target on missing": strings.Replace(good, `{ number: "14/2099/ND-CP" }`, `{ key: "vn:vbpl:1", anchor: "" }`, 1),
		"wrong country":         strings.Replace(validRecords, `"vn:vbpl:3"`, `"us:vbpl:3"`, 1),
		"tilde anchor":          strings.Replace(validRecords, `"dieu-5"`, `"dieu-5~2"`, 1),
		"acting required":       strings.Replace(validRecords, `, acting: { key: "vn:vbpl:2", anchor: "dieu-30/khoan-2" }`, "", 1),
		"acting forbidden":      strings.Replace(validRecords, `claim: { code: "reported" }`, `claim: { code: "reported", acting: { key: "vn:vbpl:2", anchor: "" } }`, 1),
		"issue id not hash":     strings.Replace(validRecords, strings.Repeat("0", 63)+"1", "abc", 1),
		"duplicate pair":        good + good,
		"out of order":          validRecords[strings.Index(validRecords, "- origin: \"issue_report\""):] + good,
		"alias":                 "- &a { origin: eval }\n- *a\n",
		"tag":                   "- !!map { origin: eval }\n",
		"two documents":         validRecords + "---\n[]\n",
		"not a list":            "origin: eval\n",
		"integer id":            strings.Replace(good, `"case-2099-1"`, `12`, 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("vn", []byte(data))
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestParse_RefusesTooMany(t *testing.T) {
	var b strings.Builder
	for i := 0; i <= MaxRecords; i++ {
		b.WriteString("- {}\n")
	}
	if _, err := Parse("vn", []byte(b.String())); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("err = %v", err)
	}
}

func TestEncode_RoundTrips(t *testing.T) {
	file, err := Parse("vn", []byte(validRecords))
	if err != nil {
		t.Fatal(err)
	}
	data := Encode(file.Records)
	if string(data) != validRecords {
		t.Fatalf("encoded:\n%s", data)
	}
	if string(Encode(nil)) != "[]\n" {
		t.Fatalf("empty = %q", Encode(nil))
	}
}

func TestSourcePath(t *testing.T) {
	if p, err := SourcePath("vn"); err != nil || p != "sources/vn-reports.yaml" {
		t.Fatalf("%q %v", p, err)
	}
	if _, err := SourcePath("../x"); err == nil {
		t.Fatal("invalid country accepted")
	}
}

func TestMerge_ReplayChangesNothing(t *testing.T) {
	file, _ := Parse("vn", []byte(validRecords))
	merged, added, _, err := Merge("vn", file, file.Records)
	if err != nil || len(added) != 0 || string(Encode(merged)) != validRecords {
		t.Fatalf("added=%d err=%v", len(added), err)
	}
	extra := Record{Origin: "eval", OriginID: "case-2099-0", Kind: "wrong_text", Target: Target{Key: "vn:vbpl:9"}, Claim: Claim{Code: "reported"}}
	merged, added, _, err = Merge("vn", file, []Record{extra, file.Records[0]})
	if err != nil || len(added) != 1 || len(merged) != 4 || merged[0].OriginID != "case-2099-0" {
		t.Fatalf("merged=%d added=%d err=%v", len(merged), len(added), err)
	}
	clash := file.Records[0]
	clash.Kind = "wrong_text"
	clash.Target = Target{Key: "vn:vbpl:9"}
	if _, _, _, err = Merge("vn", file, []Record{clash}); err == nil {
		t.Fatal("same pair with another content merged")
	}
}

func TestMerge_TakesTheFirstBoundInOrderAndReplayContinues(t *testing.T) {
	var add []Record
	for i := MaxNewPerImport + 3; i > 0; i-- {
		add = append(add, Record{Origin: "eval", OriginID: fmt.Sprintf("case-%04d", i), Kind: "wrong_text", Target: Target{Key: "vn:vbpl:9"}, Claim: Claim{Code: "reported"}})
	}
	merged, added, remaining, err := Merge("vn", File{}, add)
	if err != nil || len(added) != MaxNewPerImport || remaining != 3 || len(merged) != MaxNewPerImport {
		t.Fatalf("added=%d remaining=%d err=%v", len(added), remaining, err)
	}
	if added[0].OriginID != "case-0001" || added[MaxNewPerImport-1].OriginID != fmt.Sprintf("case-%04d", MaxNewPerImport) {
		t.Fatalf("not the first in origin-id order: %s .. %s", added[0].OriginID, added[MaxNewPerImport-1].OriginID)
	}
	merged, added, remaining, err = Merge("vn", File{Records: merged}, add)
	if err != nil || len(added) != 3 || remaining != 0 || len(merged) != MaxNewPerImport+3 {
		t.Fatalf("replay: added=%d remaining=%d err=%v", len(added), remaining, err)
	}
}

func TestMerge_ValidatesEveryRecord(t *testing.T) {
	bad := Record{Origin: "eval", OriginID: "case-1", Kind: "wrong_text", Target: Target{Key: "us:vbpl:1"}, Claim: Claim{Code: "reported"}}
	if _, _, _, err := Merge("vn", File{}, []Record{bad}); err == nil {
		t.Fatal("an invalid record was merged")
	}
}

func TestParse_RefusesNonCanonicalSpelling(t *testing.T) {
	cases := map[string]string{
		"comment":        "# note\n" + validRecords,
		"trailing":       validRecords + "# end\n",
		"unquoted":       strings.Replace(validRecords, `"eval"`, "eval", 1),
		"block mapping":  strings.Replace(validRecords, `target: { number: "14/2099/ND-CP" }`, "target:\n    number: \"14/2099/ND-CP\"", 1),
		"no anchor":      strings.Replace(validRecords, `target: { key: "vn:vbpl:3", anchor: "" }`, `target: { key: "vn:vbpl:3" }`, 1),
		"no final break": strings.TrimSuffix(validRecords, "\n"),
		"crlf":           strings.ReplaceAll(validRecords, "\n", "\r\n"),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("vn", []byte(data)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestParse_RefusesFreeTextInKeysAndNumbers(t *testing.T) {
	for name, replace := range map[string][2]string{
		"email key":     {`"vn:vbpl:3"`, `"vn:nguyenvana@gmail.com"`},
		"email in key":  {`"vn:vbpl:3"`, `"vn:vbpl:a@b.c"`},
		"sentence key":  {`"vn:vbpl:3"`, `"vn:vbpl:the law is wrong here"`},
		"other source":  {`"vn:vbpl:3"`, `"vn:other:3"`},
		"long key":      {`"vn:vbpl:3"`, `"vn:vbpl:` + strings.Repeat("a", 65) + `"`},
		"newline in id": {`"case-2099-1"`, `"case-2099-1\n"`},
		"digitless":     {`"14/2099/ND-CP"`, `"ND-CP"`},
		"long number":   {`"14/2099/ND-CP"`, `"` + strings.Repeat("1", 129) + `"`},
		"long anchor":   {`"dieu-5"`, `"dieu-` + strings.Repeat("1", 70) + `"`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("vn", []byte(strings.Replace(validRecords, replace[0], replace[1], 1))); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestParse_AcceptsWhatTheReleaseHolds(t *testing.T) {
	file := File{Records: []Record{
		{Origin: "eval", OriginID: "a-1", Kind: "missing_document", Target: Target{Number: "12/2099/QÐ-UBND & 3"}, Claim: Claim{Code: "missing_document"}},
		{Origin: "eval", OriginID: "a-2", Kind: "wrong_text", Target: Target{Key: "vn:vbpl:hhtquyetdinh_65", Anchor: "dieu-5/diem-đ"}, Claim: Claim{Code: "reported"}},
	}}
	if _, err := Parse("vn", Encode(file.Records)); err != nil {
		t.Fatal(err)
	}
}

func TestParse_RefusesOversizeFile(t *testing.T) {
	_, err := Parse("vn", append([]byte("[]\n"), make([]byte, MaxFileBytes)...))
	if err == nil || !strings.Contains(err.Error(), "exceeds 8 MiB") {
		t.Fatalf("err = %v", err)
	}
	path := filepath.Join(t.TempDir(), "vn-reports.yaml")
	if err = os.WriteFile(path, append([]byte("[]\n"), make([]byte, MaxFileBytes)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ParseFile("vn", path); err == nil || !strings.Contains(err.Error(), "exceeds 8 MiB") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseFile_MissingAndOversize(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := ParseFile("vn", filepath.Join(dir, "none.yaml")); err == nil {
		t.Fatal("missing file accepted")
	}
	path := filepath.Join(dir, "vn-reports.yaml")
	if err := os.WriteFile(path, []byte(validRecords), 0o600); err != nil {
		t.Fatal(err)
	}
	file, data, err := ParseFile("vn", path)
	if err != nil || file.Len() != 3 || string(data) != validRecords {
		t.Fatalf("err=%v", err)
	}
}

func TestSources_EveryReportsFileParses(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "sources", "*-reports.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range paths {
		country := strings.TrimSuffix(filepath.Base(path), "-reports.yaml")
		found = found || country == "vn"
		if _, _, err := ParseFile(country, path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	if !found {
		t.Fatal("sources/vn-reports.yaml is missing")
	}
}

func TestValidate_RefusesFreeTextControlAndNoncharacters(t *testing.T) {
	missing := func(number string) Record {
		return Record{Origin: "eval", OriginID: "a-1", Kind: "missing_document", Target: Target{Number: number}, Claim: Claim{Code: "missing_document"}}
	}
	text := func(anchor string) Record {
		return Record{Origin: "eval", OriginID: "a-1", Kind: "wrong_text", Target: Target{Key: "vn:vbpl:1", Anchor: anchor}, Claim: Claim{Code: "reported"}}
	}
	for name, r := range map[string]Record{
		"number email":         missing("NGUYENVANA1@GMAIL.COM"),
		"number sentence":      missing("CONG TY ABC HOI VE HOP DONG SO 12 CUA KHACH HANG NGUYEN VAN A"),
		"number at sign":       missing("12/2099/ND-CP@X"),
		"number control":       missing("12/2099/ND\x01CP"),
		"number newline":       missing("12/2099/ND\nCP"),
		"number noncharacter":  missing("12/2099/ND\uFFFECP"),
		"number noncharacter2": missing("12/2099/ND\uFDD0CP"),
		"number short year":    missing("12/99/ND-CP"),
		"number long segment":  missing("12/2099/" + strings.Repeat("A", 33)),
		"number five segments": missing("12/2099/A-B-C-D-E"),
		"number empty segment": missing("12/2099/A--B"),
		"number double space":  missing("12/2099/A  B"),
		"anchor email":         text("dieu-x@y.z"),
		"anchor email in kind": text("x-nguyenvana@gmail.com"),
		"anchor control":       text("dieu-1\x01"),
		"anchor newline":       text("dieu-1\n"),
		"anchor noncharacter":  text("dieu-1\uFFFF"),
		"anchor underscore":    text("dieu-a_b"),
	} {
		if problems := r.validate("vn"); len(problems) == 0 {
			t.Errorf("%s accepted", name)
		}
	}
	for name, r := range map[string]Record{
		"plain":       missing("12/2099/ND-CP"),
		"spaced":      missing("12/2099/Q\u00d0-UBND & 3"),
		"dotted":      missing("1/2099/TT.BTC"),
		"slashed":     missing("12/2099/ND/CP"),
		"underscore":  missing("25/2016/TT_NHNN"),
		"underscore2": missing("08/2007/TTLT-NHNN_BCA-BTP"),
		"anchor":      text("dieu-5/khoan-2.a/diem-\u0111"),
	} {
		if problems := r.validate("vn"); len(problems) != 0 {
			t.Errorf("%s refused: %v", name, problems)
		}
	}
}
