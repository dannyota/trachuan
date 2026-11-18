package evidence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

const reportRecords = `- origin: "eval"
  origin_id: "case-2099-1"
  kind: "wrong_text"
  target: { key: "vn:vbpl:1", anchor: "dieu-2" }
  claim: { code: "reported" }
- origin: "eval"
  origin_id: "case-2099-2"
  kind: "wrong_validity"
  target: { key: "vn:vbpl:1", anchor: "" }
  claim: { code: "repealed_by", acting: { key: "vn:vbpl:2", anchor: "dieu-1" } }
`

func reportsBuildDir(t *testing.T, listed bool, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	manifest := ""
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, reportsFile), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if listed {
			manifest = bareSHA256([]byte(body)) + "  " + reportsFile + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, bareSHA256([]byte(manifest))
}

func detectIn(t *testing.T, listed bool, body string) (Observation, error) {
	t.Helper()
	dir, sum := reportsBuildDir(t, listed, body)
	return detectReports("vn", dir, sum)
}

func TestReportDetector_OneFindingPerRecord(t *testing.T) {
	obs, err := detectIn(t, true, reportRecords)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Findings) != 2 || obs.Observed != 2 || obs.NotObservable != 0 {
		t.Fatalf("observation = %+v", obs)
	}
	f := obs.Findings[1]
	if f.Subject.Type != "report" || strings.Join(f.Subject.Fields, "|") != "eval|case-2099-2" || f.Discriminator != "none" {
		t.Fatalf("finding = %+v", f)
	}
	if err = f.Subject.validate(); err != nil {
		t.Fatal(err)
	}
	if !obs.ObservedCompletely(f.Subject) || !obs.ObservedCompletely(Subject{Type: "report", Fields: []string{"eval", "gone"}}) {
		t.Fatal("a removed record must read as gone, not carried")
	}
}

func TestReportDetector_FingerprintFollowsEveryContentField(t *testing.T) {
	base, _ := detectIn(t, true, reportRecords)
	want, _ := hashValue(base.Findings[1].FingerprintInputs)
	for name, replace := range map[string][2]string{
		"kind":          {`kind: "wrong_validity"`, `kind: "wrong_text"`},
		"target key":    {`target: { key: "vn:vbpl:1", anchor: "" }`, `target: { key: "vn:vbpl:9", anchor: "" }`},
		"target anchor": {`target: { key: "vn:vbpl:1", anchor: "" }`, `target: { key: "vn:vbpl:1", anchor: "dieu-1" }`},
		"claim code":    {`repealed_by`, `replaced_by`},
		"acting key":    {`key: "vn:vbpl:2"`, `key: "vn:vbpl:3"`},
		"acting anchor": {`anchor: "dieu-1" }`, `anchor: "dieu-2" }`},
	} {
		obs, err := detectIn(t, true, strings.Replace(reportRecords, replace[0], replace[1], 1))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, _ := hashValue(obs.Findings[1].FingerprintInputs); got == want {
			t.Errorf("%s does not reach the fingerprint", name)
		}
		if findingKeyOf(t, obs.Findings[1]) != findingKeyOf(t, base.Findings[1]) {
			t.Errorf("%s changed the finding key", name)
		}
	}
	number := `- origin: "eval"
  origin_id: "case-2099-3"
  kind: "missing_document"
  target: { number: "14/2099/ND-CP" }
  claim: { code: "missing_document" }
`
	a, _ := detectIn(t, true, reportRecords+number)
	b, _ := detectIn(t, true, reportRecords+strings.Replace(number, "14/2099", "15/2099", 1))
	if x, _ := hashValue(a.Findings[2].FingerprintInputs); x == "" || x == func() string { y, _ := hashValue(b.Findings[2].FingerprintInputs); return y }() {
		t.Error("a number target does not reach the fingerprint")
	}
}

func findingKeyOf(t *testing.T, f Fired) string {
	t.Helper()
	key, err := findingKey("external_report", f)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestReportDetector_NoFileMeansNoFindings(t *testing.T) {
	obs, err := detectIn(t, false, "")
	if err != nil || len(obs.Findings) != 0 || obs.ObservedCompletely == nil {
		t.Fatalf("obs=%+v err=%v", obs, err)
	}
}

func TestReportDetector_RefusesUnverifiedOrInvalid(t *testing.T) {
	if _, err := detectIn(t, false, reportRecords); err == nil {
		t.Fatal("a file the manifest does not list was read")
	}
	dir, sum := reportsBuildDir(t, true, reportRecords)
	if err := os.WriteFile(filepath.Join(dir, reportsFile), []byte(reportRecords+"# edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := detectReports("vn", dir, sum); err == nil {
		t.Fatal("a file that differs from the manifest was read")
	}
	if _, err := detectIn(t, true, "- { origin: x }\n"); err == nil {
		t.Fatal("an invalid file was read")
	}
	dir, _ = reportsBuildDir(t, true, reportRecords)
	if _, err := detectReports("vn", dir, bareSHA256([]byte("another manifest"))); err == nil {
		t.Fatal("a manifest that is not the run's build was read")
	}
}

func TestReportDetector_DetectResolvesTheBuildForCounting(t *testing.T) {
	dir, sum := reportsBuildDir(t, true, reportRecords)
	d := reportDetector{}
	run := stage.Run{Country: "vn"}
	obs, err := d.Detect(context.Background(), Inputs{Run: run, Build: func() (string, string, error) { return dir, sum, nil }})
	if err != nil || len(obs.Findings) != 2 {
		t.Fatalf("obs=%+v err=%v", obs, err)
	}
	if _, err = d.Detect(context.Background(), Inputs{Run: run}); err == nil {
		t.Fatal("no build location was accepted")
	}
	if _, err = d.Detect(context.Background(), Inputs{Run: run, BuildDir: dir}); err == nil {
		t.Fatal("a build directory without its expected hash was accepted")
	}
	wantErr := errors.New("no build")
	if _, err = d.Detect(context.Background(), Inputs{Run: run, Build: func() (string, string, error) { return "", "", wantErr }}); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v", err)
	}
	counts := CountFindings(context.Background(), Inputs{Run: run, Build: func() (string, string, error) { return dir, sum, nil }}, []Detector{d})
	if len(counts) != 1 || counts[0].NotRun != "" || counts[0].Findings != 2 || counts[0].Observed != 2 {
		t.Fatalf("counts = %+v", counts)
	}
}

func TestReportDetector_Registered(t *testing.T) {
	found := false
	for _, d := range CrossSignal() {
		found = found || d.Spec().ID == "external_report"
	}
	if !found {
		t.Fatal("external_report is not in the cross-signal list")
	}
}

func (f *fixture) buildWithReports(body string) {
	f.t.Helper()
	f.build = f.publish("build", bareSHA256([]byte("reports"+body)), map[string]string{"build.json": "{}", reportsFile: body})
	f.report["build_artifact_sha256"] = f.build.SHA256
	f.sync()
}

func TestProcess_ReportsFileOpensOneFindingPerRecord(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.buildWithReports(reportRecords)
	f.stage.cfg.Detectors = []Detector{reportDetector{}}
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	file := f.findings(output)
	if len(file.Findings) != 2 || file.Detectors["external_report"].Observed != 2 {
		t.Fatalf("findings = %+v detectors = %+v", file.Findings, file.Detectors)
	}
	for i, id := range []string{"case-2099-1", "case-2099-2"} {
		got := file.Findings[i]
		if got.Detector != "external_report" || got.Severity != "review" || got.Status != StatusOpen || got.Subject.Type != "report" {
			t.Fatalf("finding = %+v", got)
		}
		found := false
		for _, g := range file.Findings {
			found = found || strings.Join(g.Subject.Fields, "|") == "eval|"+id
		}
		if !found {
			t.Fatalf("no finding for %s", id)
		}
	}
}

func TestProcess_NoReportsFileOpensNothing(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.stage.cfg.Detectors = []Detector{reportDetector{}}
	_, output, err := f.process()
	if err != nil {
		t.Fatal(err)
	}
	if file := f.findings(output); len(file.Findings) != 0 || !file.Detectors["external_report"].Complete {
		t.Fatalf("findings = %+v", file)
	}
}

func TestProcess_UnlistedReportsFileFailsTheStage(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.buildWithReports(reportRecords)
	if err := os.Remove(filepath.Join(f.env.Artifacts.Abs(f.build.Path), manifestFile)); err != nil {
		t.Fatal(err)
	}
	f.stage.cfg.Detectors = []Detector{reportDetector{}}
	if _, _, err := f.process(); err == nil {
		t.Fatal("a build without its manifest was read")
	}
}

func TestCountInputs_ResolvesTheBuildItem(t *testing.T) {
	f := newFixture(t, noneDecision())
	f.buildWithReports(reportRecords)
	in := CountInputs(context.Background(), f.queries, f.env, f.run, nil)
	counts := CountFindings(context.Background(), in, []Detector{reportDetector{}})
	if len(counts) != 1 || counts[0].NotRun != "" || counts[0].Findings != 2 || counts[0].Observed != 2 {
		t.Fatalf("counts = %+v", counts)
	}
	dir, sum, err := in.Build()
	if err != nil || dir != f.env.Artifacts.Abs(f.build.Path) || sum != f.build.SHA256 {
		t.Fatalf("build = %q %q %v", dir, sum, err)
	}
}
