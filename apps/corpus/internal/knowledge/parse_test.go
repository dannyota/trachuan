package knowledge

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func testHash(c string) string { return "sha256:" + strings.Repeat(c, 64) }

func validProvision(indent string) string {
	return "\n" + indent + "number_norm: \"14/2020/nd-cp\"\n" +
		indent + "text_source: \"source_tree\"\n" +
		indent + "text_sha256: \"" + testHash("a") + "\"\n" +
		indent + "heading_sha256: \"" + testHash("b") + "\"\n" +
		indent + "ancestors: [{ anchor: \"chuong-1\", heading_sha256: \"" + testHash("c") + "\" }]\n"
}

func validDecision(id string) string {
	return "- id: \"" + id + "\"\n" +
		"  kind: \"review_decision\"\n" +
		"  target: { finding: \"" + testHash("1") + "\" }\n" +
		"  claim: { reason_code: \"false_positive\", reason: \"Why it needs no change.\" }\n" +
		"  depends: { fingerprint: \"" + testHash("2") + "\", semantics: 1, first_rel: \"20991118_1\" }\n" +
		"  reviewed: { date: \"2099-11-18\", by: \"primary\", method: \"human\" }\n"
}

func validEffect(id string) string {
	return "- id: \"" + id + "\"\n" +
		"  kind: \"provision_effect\"\n" +
		"  target: { key: \"vn:vbpl:1\", anchor: \"dieu-5\" }\n" +
		"  claim:\n" +
		"    acting: { key: \"vn:vbpl:2\", anchor: \"dieu-30/khoan-2\" }\n" +
		"    relation_kind: \"repeals\"\n" +
		"  evidence: { quote: \"exact text of the acting provision\" }\n" +
		"  depends:\n" +
		"    contract: 1\n" +
		"    acting:" + validProvision("      ") +
		"    target:" + validProvision("      ") +
		"  reviewed:\n" +
		"    date: \"2099-11-18\"\n" +
		"    by: \"primary\"\n" +
		"    method: \"model-adjudicated\"\n" +
		"    batch: \"effects-2099-11\"\n" +
		"    sample_read: 20\n" +
		"    sample_of: 200\n"
}

func validRelation(id string) string {
	return "- id: \"" + id + "\"\n" +
		"  kind: \"relation\"\n" +
		"  target: { key: \"vn:vbpl:1\" }\n" +
		"  claim: { acting: { key: \"vn:vbpl:2\" }, relation_kind: \"replaces\" }\n" +
		"  evidence: { anchor: \"dieu-40\", quote: \"exact text of that provision\" }\n" +
		"  depends:\n" +
		"    contract: 1\n" +
		"    provision:" + validProvision("      ") +
		"    target_number_norm: \"14/2020/nd-cp\"\n" +
		"  reviewed: { date: \"2099-11-18\", by: \"owner\", method: \"human\" }\n"
}

func validFile() string {
	return validDecision("decision-mau") + validEffect("effect-mau") + validRelation("relation-mau")
}

func TestParse_ValidFile(t *testing.T) {
	file, err := Parse("vn", []byte(validFile()))
	if err != nil {
		t.Fatal(err)
	}
	if file.Len() != 3 || len(file.Decisions) != 1 || len(file.Claims) != 2 {
		t.Fatalf("file = %+v", file)
	}
	d := file.Decisions[0]
	if d.ID != "decision-mau" || d.Position != 0 || d.Finding != testHash("1") || d.ReasonCode != "false_positive" || d.Fingerprint != testHash("2") || d.Semantics != 1 || d.FirstRel != "20991118_1" || d.Reviewed.By != "primary" {
		t.Fatalf("decision = %+v", d)
	}
	effect := file.Claims[0]
	if effect.Kind != KindEffect || effect.Position != 1 || effect.ActingKey != "vn:vbpl:2" || effect.ActingAnchor != "dieu-30/khoan-2" || effect.TargetKey != "vn:vbpl:1" || effect.TargetAnchor != "dieu-5" || effect.RelationKind != "repeals" || effect.Contract != 1 || effect.TargetNumberNorm != "14/2020/nd-cp" {
		t.Fatalf("effect = %+v", effect)
	}
	if effect.ActingDepends.TextSHA256 != testHash("a") || len(effect.TargetDepends.Ancestors) != 1 || effect.TargetDepends.Ancestors[0] != (Ancestor{"chuong-1", testHash("c")}) {
		t.Fatalf("effect depends = %+v", effect)
	}
	if effect.Reviewed.Batch != "effects-2099-11" || effect.Reviewed.SampleRead != 20 || effect.Reviewed.SampleOf != 200 {
		t.Fatalf("reviewed = %+v", effect.Reviewed)
	}
	relation := file.Claims[1]
	if relation.Kind != KindRelation || relation.Position != 2 || relation.ActingAnchor != "dieu-40" || relation.TargetAnchor != "" || relation.TargetNumberNorm != "14/2020/nd-cp" || relation.Quote != "exact text of that provision" {
		t.Fatalf("relation = %+v", relation)
	}
}

func TestParse_NoEntries(t *testing.T) {
	for _, data := range []string{"", "\n", "# only a comment\n", "[]\n", "# c\n[]\n"} {
		file, err := Parse("vn", []byte(data))
		if err != nil || file.Len() != 0 {
			t.Errorf("Parse(%q) = %+v, %v", data, file, err)
		}
	}
}

func replaced(old, new string) func(string) string {
	return func(s string) string {
		if !strings.Contains(s, old) {
			panic("fixture lacks " + old)
		}
		return strings.Replace(s, old, new, 1)
	}
}

func TestParse_Refusals(t *testing.T) {
	for name, tc := range map[string]struct {
		edit func(string) string
		want string
	}{
		"not a list":       {func(string) string { return "a: 1\n" }, "must be one list"},
		"null document":    {func(string) string { return "---\n" }, "must be one list"},
		"further document": {func(s string) string { return s + "---\n[]\n" }, "further documents"},
		"anchor":           {replaced(`- id: "effect-mau"`, `- &x id: "effect-mau"`), "anchors"},
		"alias": {func(s string) string {
			return replaced(`kind: "relation"`, "kind: *x")(replaced(`kind: "review_decision"`, `kind: &x "review_decision"`)(s))
		}, "aliases"},
		"merge key":              {replaced(`  kind: "relation"`, "  <<: { a: 1 }\n  kind: \"relation\""), "merge"},
		"unknown entry field":    {replaced(`  kind: "relation"`, "  url: \"https://x\"\n  kind: \"relation\""), `unknown field "url"`},
		"unknown nested field":   {replaced(`evidence: { quote:`, `evidence: { url: "x", quote:`), `unknown field "url"`},
		"decision evidence":      {replaced(`  kind: "review_decision"`, "  evidence: { quote: \"0123456789\" }\n  kind: \"review_decision\""), `unknown field "evidence"`},
		"duplicate field":        {replaced(`  kind: "relation"`, "  kind: \"relation\"\n  kind: \"relation\""), "duplicate field"},
		"missing field":          {replaced("  reviewed: { date: \"2099-11-18\", by: \"owner\", method: \"human\" }\n", ""), `missing field "reviewed"`},
		"unknown kind":           {replaced(`kind: "relation"`, `kind: "other"`), "kind"},
		"unquoted date":          {replaced(`date: "2099-11-18", by: "owner"`, `date: 2099-11-18, by: "owner"`), "must be a string"},
		"unquoted first_rel":     {replaced(`first_rel: "20991118_1"`, `first_rel: 20991118_1`), "must be a string"},
		"string contract":        {replaced("    contract: 1\n    acting:", "    contract: \"1\"\n    acting:"), "decimal integer"},
		"zero contract":          {replaced("    contract: 1\n    acting:", "    contract: 0\n    acting:"), "contract"},
		"empty id":               {replaced(`id: "effect-mau"`, `id: ""`), "invalid id"},
		"single-quoted empty id": {replaced(`id: "effect-mau"`, `id: ''`), "invalid id"},
		"two empty ids": {func(string) string {
			return replaced(`id: "relation-a"`, `id: ""`)(validRelation("relation-a")) + replaced(`id: "effect-a"`, `id: ""`)(validEffect("effect-a"))
		}, "invalid id"},
		"unquoted id":           {replaced(`id: "effect-mau"`, `id: 12345`), "must be a string"},
		"bad id":                {replaced(`"effect-mau"`, `"Effect"`), "id"},
		"short id":              {replaced(`"effect-mau"`, `"ef"`), "id"},
		"out of order":          {func(s string) string { return validEffect("zzz-last") + validRelation("aaa-first") }, "id order"},
		"duplicate id":          {func(string) string { return validRelation("same-id") + validRelation("same-id") }, "duplicate id"},
		"foreign key":           {replaced(`target: { key: "vn:vbpl:1" }`, `target: { key: "th:vbpl:1" }`), "key"},
		"bare country key":      {replaced(`target: { key: "vn:vbpl:1" }`, `target: { key: "vn:" }`), "key"},
		"bad hash":              {replaced(`finding: "sha256:1111`, `finding: "sha256:ABCD`), "sha256"},
		"short hash":            {replaced(`fingerprint: "`+testHash("2"), `fingerprint: "sha256:22`), "sha256"},
		"bad rel":               {replaced(`first_rel: "20991118_1"`, `first_rel: "2099"`), "first_rel"},
		"bad date":              {replaced(`date: "2099-11-18", by: "owner"`, `date: "2099-13-18", by: "owner"`), "date"},
		"bad reason code":       {replaced(`"false_positive"`, `"because"`), "reason_code"},
		"empty reason":          {replaced(`reason: "Why it needs no change."`, `reason: ""`), "reason"},
		"long reason":           {replaced(`reason: "Why it needs no change."`, `reason: "`+strings.Repeat("x", 201)+`"`), "reason"},
		"zero semantics":        {replaced("semantics: 1", "semantics: 0"), "semantics"},
		"short quote":           {replaced(`"exact text of that provision"`, `"too short"`), "quote"},
		"long quote":            {replaced(`"exact text of that provision"`, `"`+strings.Repeat("x", 201)+`"`), "quote"},
		"effect kind":           {replaced(`relation_kind: "repeals"`, `relation_kind: "cites"`), "relation_kind"},
		"relation unknown":      {replaced(`relation_kind: "replaces"`, `relation_kind: "unknown"`), "relation_kind"},
		"relation consolidates": {replaced(`relation_kind: "replaces"`, `relation_kind: "consolidates"`), "relation_kind"},
		"same document effect":  {replaced(`acting: { key: "vn:vbpl:2", anchor: "dieu-30/khoan-2" }`, `acting: { key: "vn:vbpl:1", anchor: "dieu-30/khoan-2" }`), "documents must differ"},
		"same provision":        {replaced(`acting: { key: "vn:vbpl:2", anchor: "dieu-30/khoan-2" }`, `acting: { key: "vn:vbpl:1", anchor: "dieu-5" }`), "differ"},
		"same document":         {replaced(`acting: { key: "vn:vbpl:2" }`, `acting: { key: "vn:vbpl:1" }`), "differ"},
		"suffixed target":       {replaced(`anchor: "dieu-5"`, `anchor: "dieu-5~2"`), "~k"},
		"suffixed acting":       {replaced(`anchor: "dieu-30/khoan-2"`, `anchor: "dieu-30~2/khoan-2"`), "~k"},
		"suffixed evidence":     {replaced(`anchor: "dieu-40"`, `anchor: "dieu-40~3"`), "~k"},
		"suffixed ancestor":     {replaced(`anchor: "chuong-1"`, `anchor: "chuong-1~2"`), "~k"},
		"malformed anchor":      {replaced(`anchor: "dieu-40"`, `anchor: "dieu-40/"`), "anchor"},
		"upper-case anchor":     {replaced(`anchor: "dieu-40"`, `anchor: "Dieu-40"`), "anchor"},
		"bad by":                {replaced(`by: "owner"`, `by: "someone"`), "by"},
		"bad method":            {replaced(`by: "owner", method: "human"`, `by: "owner", method: "guess"`), "method"},
		"human with batch":      {replaced(`by: "owner", method: "human"`, `by: "owner", method: "human", batch: "b-1"`), `unknown field "batch"`},
		"model without batch":   {replaced("    batch: \"effects-2099-11\"\n", ""), `missing field "batch"`},
		"bad batch":             {replaced(`batch: "effects-2099-11"`, `batch: "B"`), "batch"},
		"zero sample":           {replaced("sample_read: 20", "sample_read: 0"), "sample"},
		"read above total":      {replaced("sample_read: 20", "sample_read: 201"), "sample"},
		"under ten percent":     {replaced("sample_read: 20", "sample_read: 19"), "sample"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("vn", []byte(tc.edit(validFile())))
			var invalid *ValidationError
			if !errors.As(err, &invalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParse_RefusesExplicitTags(t *testing.T) {
	for name, edit := range map[string]func(string) string{
		"core string tag":    replaced(`first_rel: "20991118_1"`, `first_rel: !!str 20991118_1`),
		"core integer tag":   replaced("semantics: 1", "semantics: !!int 1"),
		"local tag":          replaced(`by: "owner"`, `by: !who "owner"`),
		"non-specific tag":   replaced(`by: "owner"`, `by: ! "owner"`),
		"non-specific plain": replaced("semantics: 1", "semantics: ! 1"),
		"after wide text":    replaced(`claim: { reason_code: "false_positive", reason: "Why it needs no change." }`, `claim: { reason: "Vì ệ ệ ệ", reason_code: ! "false_positive" }`),
		"mapping tag":        replaced(`target: { key: "vn:vbpl:1" }`, `target: !!map { key: "vn:vbpl:1" }`),
		"sequence tag":       replaced(`ancestors: [{`, `ancestors: !!seq [{`),
		"root tag":           func(s string) string { return "--- !!seq\n" + s },
		"tag directive": func(s string) string {
			return "%TAG ! tag:example.org:\n---\n" + replaced(`by: "owner"`, `by: !who "owner"`)(s)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse("vn", []byte(edit(validFile()))); err == nil || !strings.Contains(err.Error(), "explicit tags") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	data := replaced(`reason: "Why it needs no change."`, `reason: "! not a tag, only text"`)(validFile())
	if _, err := Parse("vn", []byte(data)); err != nil {
		t.Fatalf("quoted exclamation refused: %v", err)
	}
}

func TestParse_IntegersArePlainDecimal(t *testing.T) {
	fields := map[string]func(string) func(string) string{
		"contract": func(v string) func(string) string {
			return replaced("    contract: 1\n    acting:", "    contract: "+v+"\n    acting:")
		},
		"semantics":   func(v string) func(string) string { return replaced("semantics: 1", "semantics: "+v) },
		"sample_read": func(v string) func(string) string { return replaced("sample_read: 20", "sample_read: "+v) },
		"sample_of":   func(v string) func(string) string { return replaced("sample_of: 200", "sample_of: "+v) },
	}
	for field, edit := range fields {
		for _, value := range []string{"0", "00", "01", "0x1", "0o7", "0b1", "1_0", "+1", "-1", "1.0", "1e1", `"1"`, "99999999999999999999"} {
			t.Run(field+" "+value, func(t *testing.T) {
				if _, err := Parse("vn", []byte(edit(value)(validFile()))); err == nil || !strings.Contains(err.Error(), field) {
					t.Fatalf("err = %v", err)
				}
			})
		}
	}
	if _, err := Parse("vn", []byte(fields["contract"]("2")(validFile()))); err != nil {
		t.Fatalf("a later contract must parse and go stale in the run checks: %v", err)
	}
}

func TestParse_OnlyValidIDsEnterOrderChecks(t *testing.T) {
	for name, tc := range map[string]struct{ first, want string }{
		"boolean id": {"id: true", "must be a string"},
		"integer id": {"id: 99999", "must be a string"},
		"invalid id": {`id: "zzz!"`, "invalid id"},
	} {
		t.Run(name, func(t *testing.T) {
			data := replaced(`id: "relation-z"`, tc.first)(validRelation("relation-z")) + validEffect("effect-a")
			_, err := Parse("vn", []byte(data))
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "id order") || strings.Contains(err.Error(), "duplicate id") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("invalid ids do not count as duplicates", func(t *testing.T) {
		data := replaced(`"relation-a"`, `"BAD"`)(validRelation("relation-a")) + replaced(`"relation-b"`, `"BAD"`)(validRelation("relation-b"))
		if _, err := Parse("vn", []byte(data)); err == nil || strings.Contains(err.Error(), "duplicate id") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestParse_QuoteCountsCodePoints(t *testing.T) {
	ten := strings.Repeat("ệ", 10)
	data := replaced(`"exact text of that provision"`, `"`+ten+`"`)(validFile())
	if _, err := Parse("vn", []byte(data)); err != nil {
		t.Fatalf("ten code points refused: %v", err)
	}
	data = replaced(`"exact text of that provision"`, `"`+strings.Repeat("ệ", 200)+`"`)(validFile())
	if _, err := Parse("vn", []byte(data)); err != nil {
		t.Fatalf("200 code points refused: %v", err)
	}
}

func TestParse_CrossEntryRules(t *testing.T) {
	kind := replaced(`relation_kind: "repeals"`, `relation_kind: "suspends"`)
	otherFinding := replaced(testHash("1"), testHash("3"))
	otherBatch := replaced("sample_of: 200", "sample_of: 100")
	for name, tc := range map[string]struct {
		data, want string
	}{
		"effect identity":     {validEffect("effect-a") + validEffect("effect-b"), "same identity"},
		"effect contradicted": {validEffect("effect-a") + kind(validEffect("effect-b")), "different kinds"},
		"relation identity":   {validRelation("relation-a") + validRelation("relation-b"), "same identity"},
		"decision identity":   {validDecision("decision-a") + validDecision("decision-b"), "same decision"},
		"batch disagrees":     {validEffect("effect-a") + otherBatch(replaced(`anchor: "dieu-5"`, `anchor: "dieu-6"`)(validEffect("effect-b"))), "batch"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("vn", []byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("distinct decisions pass", func(t *testing.T) {
		if _, err := Parse("vn", []byte(validDecision("decision-a")+otherFinding(validDecision("decision-b")))); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("batch larger than its sample total", func(t *testing.T) {
		var b strings.Builder
		small := replaced("sample_of: 200", "sample_of: 2")
		read := replaced("sample_read: 20", "sample_read: 1")
		for i := 0; i < 3; i++ {
			b.WriteString(read(small(replaced(`anchor: "dieu-5"`, `anchor: "dieu-`+strconv.Itoa(i+10)+`"`)(validEffect("effect-" + strconv.Itoa(100+i))))))
		}
		if _, err := Parse("vn", []byte(b.String())); err == nil || !strings.Contains(err.Error(), "batch") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestParse_Bounds(t *testing.T) {
	if _, err := Parse("vn", []byte(strings.Repeat("#", MaxFileBytes+1))); err == nil || !strings.Contains(err.Error(), "MiB") {
		t.Fatalf("oversized file: %v", err)
	}
	var b strings.Builder
	for i := 0; i <= MaxEntries; i++ {
		b.WriteString("- {}\n")
	}
	if _, err := Parse("vn", []byte(b.String())); err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("too many entries: %v", err)
	}
	if _, err := Parse("VN", nil); err == nil {
		t.Fatal("invalid country accepted")
	}
}

func TestParseFile_ReturnsBytesAndWrapsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vn-knowledge.yaml")
	if _, _, err := ParseFile("vn", path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	want := validFile() + "# trailing comment\n"
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	file, data, err := ParseFile("vn", path)
	if err != nil || string(data) != want || len(file.Decisions) != 1 {
		t.Fatalf("file=%+v data=%d err=%v", file, len(data), err)
	}
}
