package evidence

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type changesCase struct {
	t                  *testing.T
	dir                string
	previous, current  *fakeSummary
	before, after      members
	predecessorBinding string
}

func newChangesCase(t *testing.T) *changesCase {
	previous, before := previousSummary()
	current, after := currentSummary()
	return &changesCase{t: t, dir: t.TempDir(), previous: previous, current: current, before: before, after: after}
}

func (c *changesCase) inputs() changeInputs {
	c.t.Helper()
	prev := writeIdentityFile(c.t, c.dir, "previous.jsonl", c.previous, c.before, identityTestBindings(previousRel))
	cur := writeIdentityFile(c.t, c.dir, "current.jsonl", c.current, c.after, identityTestBindings(testRel))
	data, err := os.ReadFile(prev)
	if err != nil {
		c.t.Fatal(err)
	}
	binding := c.predecessorBinding
	if binding == "" {
		binding = hashPrefix + bareSHA256(data)
	}
	return changeInputs{current: cur, predecessor: prev, members: c.after, bindings: changesBindings{identityBindings: identityTestBindings(testRel), PredecessorRel: previousRel, PredecessorIdentitySHA256: binding}}
}

func (c *changesCase) run() (string, error) {
	c.t.Helper()
	var buf bytes.Buffer
	err := writeChanges(&buf, c.inputs())
	return buf.String(), err
}

func (c *changesCase) lines() []string {
	c.t.Helper()
	text, err := c.run()
	if err != nil {
		c.t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func hasLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func assertLines(t *testing.T, lines []string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !hasLine(lines, w) {
			t.Errorf("missing line %s\nin\n%s", w, strings.Join(lines, "\n"))
		}
	}
}

func TestWriteChanges_Golden(t *testing.T) {
	text, err := newChangesCase(t).run()
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "changes.jsonl", []byte(text))
}

func TestWriteChanges_DocumentCauseClasses(t *testing.T) {
	lines := newChangesCase(t).lines()
	assertLines(t, lines,
		`{"type":"document","key":"vn:vbpl:2099002","change":"removed","cause":"membership"}`,
		`{"type":"document","key":"vn:vbpl:2099006","change":"removed","cause":"coverage"}`,
		`{"type":"document","key":"vn:chinhphu:2099007","change":"removed","cause":"source","replaced_by":"vn:vbpl:2099008"}`,
		`{"type":"document","key":"vn:vbpl:2099005","change":"added","cause":"coverage"}`,
		`{"type":"document","key":"vn:vbpl:2099009","change":"added","cause":"membership"}`,
		`{"type":"document","key":"vn:vbpl:2099010","change":"added","cause":"source"}`,
	)
	for _, line := range lines {
		if strings.Contains(line, `"key":"vn:chinhphu:2099003"`) || strings.Contains(line, `"key":"vn:vbpl:2099004","change"`) {
			t.Errorf("hand-over listed as removal or addition: %s", line)
		}
	}
}

func TestWriteChanges_SurvivingDocumentRecords(t *testing.T) {
	lines := newChangesCase(t).lines()
	assertLines(t, lines,
		`{"type":"document_metadata","key":"vn:vbpl:2099001","number_norm":{"before":"01/2099/TT-NHNN","after":"1/2099/TT-NHNN"}}`,
		`{"type":"document_legal","key":"vn:vbpl:2099001","status_class":{"before":"in_force","after":"expired"},"eff_to":{"before":null,"after":"2099-06-01"},"is_current":{"before":true,"after":false}}`,
		`{"type":"document_provenance","key":"vn:vbpl:2099001","text_source":{"before":"source_tree","after":"html_transcription"}}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-1","change":"changed","text_sha256":{"before":"`+textHash("Điều 1 cũ")+`","after":"`+textHash("Điều 1 mới")+`"},"provenance":true}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-1/khoan-1","change":"removed","text_sha256":{"before":"`+textHash("Khoản 1")+`","after":null},"provenance":true}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-3","change":"added","text_sha256":{"before":null,"after":"`+textHash("Điều 3")+`"},"provenance":true}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-4~2","change":"added","text_sha256":{"before":null,"after":"`+textHash("Điều 4 lặp")+`"},"provenance":true,"ambiguous":true}`,
		`{"type":"provision_classification","key":"vn:vbpl:2099001","anchor":"dieu-2","added":["1.1.b"],"removed":["1.1.a"],"provenance":true}`,
		`{"type":"provision_validity","key":"vn:vbpl:2099001","anchor":"dieu-2","added":[{"status_class":"expired","eff_from":"2099-03-01","eff_to":null,"method":"text"}],"removed":[],"provenance":true}`,
	)
	for _, line := range lines {
		if strings.Contains(line, `"anchor":"dieu-4",`) {
			t.Errorf("unchanged provision listed: %s", line)
		}
	}
}

func TestWriteChanges_HandOverIsOneSurvivingDocument(t *testing.T) {
	lines := newChangesCase(t).lines()
	assertLines(t, lines,
		`{"type":"document_key","key":"vn:vbpl:2099004","before":"vn:chinhphu:2099003"}`,
		`{"type":"document_provenance","key":"vn:vbpl:2099004","text_source":{"before":"pdf_text","after":"source_tree"},"text_complete":{"before":false,"after":true}}`,
		`{"type":"provision","key":"vn:vbpl:2099004","anchor":"dieu-2","change":"removed","text_sha256":{"before":"`+textHash("Điều 2 nghị định")+`","after":null},"provenance":true}`,
		`{"type":"provision","key":"vn:vbpl:2099004","anchor":"dieu-3","change":"added","text_sha256":{"before":null,"after":"`+textHash("Điều 3 nghị định")+`"},"provenance":true}`,
	)
	for _, line := range lines {
		if strings.Contains(line, `"kind":"cites"`) || strings.Contains(line, `"anchor":"dieu-1","change"`) && strings.Contains(line, "2099004") {
			t.Errorf("unchanged after reading the old key as the new: %s", line)
		}
	}
}

func TestWriteChanges_HandOverNeedsBothDocumentLines(t *testing.T) {
	t.Run("replacement without a line is a removal naming it", func(t *testing.T) {
		c := newChangesCase(t)
		c.current.documents = c.current.documents[:1:1]
		c.current.documents = append(c.current.documents, doc("vn:vbpl:2099005", "5/2099/TT-BTC", "in_force"))
		delete(c.current.provisions, "vn:vbpl:2099004")
		c.current.relations = c.current.relations[1:]
		assertLines(t, c.lines(), `{"type":"document","key":"vn:chinhphu:2099003","change":"removed","cause":"source","replaced_by":"vn:vbpl:2099004"}`)
	})
	t.Run("old key without a predecessor line makes the new key an addition", func(t *testing.T) {
		c := newChangesCase(t)
		c.previous.documents = append(c.previous.documents[:2:2], c.previous.documents[3:]...)
		delete(c.previous.provisions, "vn:chinhphu:2099003")
		c.previous.relations = c.previous.relations[1:]
		lines := c.lines()
		assertLines(t, lines, `{"type":"document","key":"vn:vbpl:2099004","change":"added","cause":"source"}`)
		for _, line := range lines {
			if strings.Contains(line, `"type":"document_key"`) {
				t.Errorf("hand-over paired without an old line: %s", line)
			}
		}
	})
	t.Run("two old keys naming one replacement are not paired", func(t *testing.T) {
		c := newChangesCase(t)
		c.previous.documents = append(c.previous.documents, doc("vn:chinhphu:2099012", "3/2099/ND-CP", "in_force"))
		c.after = c.after.with("vn:chinhphu:2099012", "source_replaced", "vn:vbpl:2099004")
		lines := c.lines()
		assertLines(t, lines,
			`{"type":"document","key":"vn:chinhphu:2099003","change":"removed","cause":"source","replaced_by":"vn:vbpl:2099004"}`,
			`{"type":"document","key":"vn:chinhphu:2099012","change":"removed","cause":"source","replaced_by":"vn:vbpl:2099004"}`,
			`{"type":"document","key":"vn:vbpl:2099004","change":"added","cause":"source"}`,
		)
		for _, line := range lines {
			if strings.Contains(line, `"type":"document_key"`) {
				t.Errorf("ambiguous hand-over paired: %s", line)
			}
		}
	})
	t.Run("a second old key without a predecessor line still blocks the pair", func(t *testing.T) {
		c := newChangesCase(t)
		c.after = c.after.with("vn:chinhphu:2099013", "source_replaced", "vn:vbpl:2099004")
		lines := c.lines()
		assertLines(t, lines,
			`{"type":"document","key":"vn:chinhphu:2099003","change":"removed","cause":"source","replaced_by":"vn:vbpl:2099004"}`,
			`{"type":"document","key":"vn:vbpl:2099004","change":"added","cause":"source"}`,
		)
		for _, line := range lines {
			if strings.Contains(line, `"type":"document_key"`) {
				t.Errorf("hand-over paired although two old keys name the replacement: %s", line)
			}
		}
	})
	t.Run("new key already in the predecessor is not paired", func(t *testing.T) {
		c := newChangesCase(t)
		c.previous.documents = append(c.previous.documents, doc("vn:vbpl:2099004", "3/2099/ND-CP", "in_force"))
		lines := c.lines()
		assertLines(t, lines, `{"type":"document","key":"vn:chinhphu:2099003","change":"removed","cause":"source","replaced_by":"vn:vbpl:2099004"}`)
		for _, line := range lines {
			if strings.Contains(line, `"type":"document_key"`) {
				t.Errorf("ambiguous hand-over paired: %s", line)
			}
		}
	})
}

func TestWriteChanges_NoMoves(t *testing.T) {
	c := newChangesCase(t)
	rows := c.current.provisions["vn:vbpl:2099001"]
	for i := range rows {
		if rows[i].Anchor == "dieu-3" {
			rows[i].TextSHA256 = textHash("Khoản 1")
		}
	}
	lines := c.lines()
	assertLines(t, lines,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-1/khoan-1","change":"removed","text_sha256":{"before":"`+textHash("Khoản 1")+`","after":null},"provenance":true}`,
		`{"type":"provision","key":"vn:vbpl:2099001","anchor":"dieu-3","change":"added","text_sha256":{"before":null,"after":"`+textHash("Khoản 1")+`"},"provenance":true}`,
	)
	for _, line := range lines {
		if strings.Contains(line, `"change":"moved"`) || strings.Contains(line, `"moved_from"`) {
			t.Errorf("move reported: %s", line)
		}
	}
}

func TestWriteChanges_RelationsCompareAsCounts(t *testing.T) {
	lines := newChangesCase(t).lines()
	assertLines(t, lines,
		`{"type":"relation","kind":"guided_by","method":"vbpl_diagram","source":{"key":"vn:vbpl:2099001","number":"1/2099/TT-NHNN","in_release":true,"anchor":null},"target":{"key":"vn:vbpl:2099002","number":"2/2099/TT-NHNN","in_release":true,"anchor":null},"change":"removed","count":1}`,
		`{"type":"relation","kind":"repeals","method":"text_rule/1","source":{"key":"vn:vbpl:2099001","number":"1/2099/TT-NHNN","in_release":true,"anchor":"dieu-4"},"target":{"key":null,"number":"9/2098/TT","in_release":false,"anchor":null,"named":"dieu-3"},"change":"added","count":1}`,
	)
}

func TestWriteChanges_ProvenanceOnlyWhenTextSourceOrCompletenessChanged(t *testing.T) {
	c := newChangesCase(t)
	for i := range c.current.documents {
		if c.current.documents[i].Key == "vn:vbpl:2099001" {
			c.current.documents[i].TextSource = "source_tree"
		}
	}
	for _, line := range c.lines() {
		if strings.Contains(line, `"key":"vn:vbpl:2099001","anchor"`) && strings.Contains(line, `"provenance":true`) {
			t.Errorf("provenance without a provenance change: %s", line)
		}
	}
}

func TestWriteChanges_BaselineListsNothing(t *testing.T) {
	c := newChangesCase(t)
	in := c.inputs()
	in.predecessor = ""
	in.bindings.PredecessorIdentitySHA256 = noneValue
	var buf bytes.Buffer
	if err := writeChanges(&buf, in); err != nil {
		t.Fatal(err)
	}
	want := `{"schema":1,"bindings":{"rel":"20990102_1","dump_sha256":"sha256:` + strings.Repeat("d", 64) + `","build_artifact_sha256":"sha256:` + strings.Repeat("b", 64) + `","report_sha256":"sha256:` + strings.Repeat("c", 64) + `","predecessor_rel":"20990101_1","predecessor_identity_sha256":"none"},"baseline":true}` + "\n"
	if buf.String() != want {
		t.Fatalf("got %s", buf.String())
	}
}

func TestWriteChanges_DeterministicToTheByte(t *testing.T) {
	c := newChangesCase(t)
	first, err := c.run()
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.run()
	if err != nil || first != second {
		t.Fatalf("changes differ: %v", err)
	}
}

func TestWriteChanges_RefusesUnboundOrDamagedSummaries(t *testing.T) {
	for name, mutate := range map[string]func(*changesCase, *changeInputs){
		"predecessor of another release": func(_ *changesCase, in *changeInputs) { in.bindings.PredecessorRel = "20990101_2" },
		"current of another report": func(_ *changesCase, in *changeInputs) {
			in.bindings.ReportSHA256 = hashPrefix + strings.Repeat("e", 64)
		},
		"predecessor out of order": func(c *changesCase, in *changeInputs) {
			data, err := os.ReadFile(in.predecessor)
			if err != nil {
				c.t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			lines[1], lines[len(lines)-1] = lines[len(lines)-1], lines[1]
			if err = os.WriteFile(in.predecessor, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
				c.t.Fatal(err)
			}
		},
		"predecessor record of an unknown type": func(c *changesCase, in *changeInputs) {
			appendTo(c.t, in.predecessor, `{"type":"zzz"}`+"\n")
		},
		"predecessor line not JSON": func(c *changesCase, in *changeInputs) {
			appendTo(c.t, in.predecessor, "{not json\n")
		},
		"predecessor line too long": func(c *changesCase, in *changeInputs) {
			appendTo(c.t, in.predecessor, `{"type":"relation","kind":"`+strings.Repeat("x", lineLimit)+`"}`+"\n")
		},
		"predecessor missing": func(_ *changesCase, in *changeInputs) {
			in.predecessor = filepath.Join(filepath.Dir(in.predecessor), "absent.jsonl")
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := newChangesCase(t)
			in := c.inputs()
			mutate(c, &in)
			var buf bytes.Buffer
			err := writeChanges(&buf, in)
			var refused *inputError
			if !errors.As(err, &refused) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func relationOrderKeys(t *testing.T, text, member string) []string {
	t.Helper()
	var keys []string
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if !strings.HasPrefix(line, `{"type":"relation",`) {
			continue
		}
		at := strings.Index(line, `,"`+member+`":`)
		if at < 0 || !strings.Contains(line[at:], `"count":`) || strings.Contains(line[:at], `"count":`) {
			t.Fatalf("relation line without a trailing %s member: %s", member, line)
		}
		keys = append(keys, line[:at])
	}
	return keys
}

func assertStrictlyIncreasing(t *testing.T, keys []string) {
	t.Helper()
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("relation lines out of order:\n%s\n%s", keys[i-1], keys[i])
		}
	}
}

func TestWriteChanges_RelationLinesSortByTheirBytesBeforeChange(t *testing.T) {
	c := newChangesCase(t)
	c.previous.relations = append(c.previous.relations,
		rel("cites", "vbpl_diagram", "vn:vbpl:2099001", "1/2099/TT-NHNN", nil, "vn:vbpl:2099005", "5/2099/TT-BTC", true, nil, nil),
		rel("amends_supplements", "vbpl_diagram", "vn:vbpl:2099001", "1/2099/TT-NHNN", nil, "", "8/2098/TT", false, nil, nil))
	text, err := c.run()
	if err != nil {
		t.Fatal(err)
	}
	keys := relationOrderKeys(t, text, "change")
	if len(keys) < 4 {
		t.Fatalf("too few relation changes: %d", len(keys))
	}
	assertStrictlyIncreasing(t, keys)
	golden, err := os.ReadFile(filepath.Join("testdata", changesFile))
	if err != nil {
		t.Fatal(err)
	}
	assertStrictlyIncreasing(t, relationOrderKeys(t, string(golden), "change"))
}

func TestWriteIdentity_RelationLinesSortByTheirBytesBeforeCount(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", identityFile))
	if err != nil {
		t.Fatal(err)
	}
	assertStrictlyIncreasing(t, relationOrderKeys(t, string(golden), "count"))
}
