package relate

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate/textrule"
)

func TestTextRelations_FixtureArtifactBytes(t *testing.T) {
	type fixture struct {
		Name, Text string
		Want       json.RawMessage
	}
	type output struct {
		Name              string
		Relations, Report json.RawMessage
	}
	var cases []fixture
	numbers := map[string]bool{}
	for _, name := range []string{"accept", "reject"} {
		var fixtures []fixture
		if err := readJSON(filepath.Join("textrule", "testdata", name+".json"), &fixtures); err != nil {
			t.Fatal(err)
		}
		for _, f := range fixtures {
			if name == "accept" {
				var matches []textrule.Match
				if err := json.Unmarshal(f.Want, &matches); err != nil {
					t.Fatal(err)
				}
				for _, m := range matches {
					for _, target := range m.Targets {
						numbers[target.Number] = true
					}
				}
			}
			f.Name = name + "/" + f.Name
			cases = append(cases, f)
		}
	}
	// Inserted labels are accepted under the scoped text recipe.
	for _, locator := range []string{"khoản 2a Điều 5", "điểm a1 Điều 5", "điểm a Điều 5a"} {
		cases = append(cases, fixture{Name: "inserted/" + locator, Text: "Bãi bỏ " + locator + " của Thông tư 39/2016/TT-NHNN."})
	}
	var ordered []string
	for number := range numbers {
		ordered = append(ordered, number)
	}
	sort.Strings(ordered)
	var links []relation
	expected := map[string]bool{}
	for i, number := range ordered {
		id := fmt.Sprintf("fixture-%d", i)
		links = append(links, relation{Ordinal: i, Method: "vbpl_diagram", TargetVBPLID: id, TargetNumber: number})
		expected["vn:vbpl:"+id] = true
	}
	issuers := map[string]string{"NHNN": "NGÂN HÀNG NHÀ NƯỚC VIỆT NAM", "CP": "CHÍNH PHỦ", "TTg": "THỦ TƯỚNG CHÍNH PHỦ", "BTP": "BỘ TƯ PHÁP", "QH": "QUỐC HỘI", "UBTVQH": "ỦY BAN THƯỜNG VỤ QUỐC HỘI"}
	dir := t.TempDir()
	writeTextFixture(t, dir, "report.json", []stage.Gap{})
	var outputs []output
	for _, f := range cases {
		writeTextFixture(t, dir, "provisions.json", []chunk.Node{{NodeID: f.Name, Kind: "dieu", Text: f.Text}})
		rows, report, err := textRelations(dir, append([]relation(nil), links...), issuers)
		if err != nil {
			t.Fatal(err)
		}
		rowBytes, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		fullReport := relationReport{Unmapped: []string{}, Text: report}
		reportBytes, err := json.Marshal(fullReport)
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, output{f.Name, rowBytes, reportBytes})
		// Exercise the run gap projection too; it must remain deterministic.
		gapBytes, err := json.Marshal(fullReport.resolveGaps(rows, expected))
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, output{f.Name + "/gaps", nil, gapBytes})
	}
	data, err := json.Marshal(outputs)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(data))
	// Captured after the three inserted-label cases changed from gaps to rows.
	const want = "954a9d69525669bfdd36c5298cf5b7e3d5dca524f38158ae65e02e7a51657f7b"
	if got != want {
		t.Fatalf("%d fixture outputs: SHA-256=%s want %s", len(cases), got, want)
	}
	t.Logf("%d fixture outputs and gap projections match the digest: %s", len(cases), got)
}
