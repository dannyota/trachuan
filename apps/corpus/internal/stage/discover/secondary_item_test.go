package discover

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/chinhphu"
	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func secondaryFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "chinhphu", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func secondaryRun(t *testing.T, vbplResponse, detail, key string) (stage.Resolution, map[string]any, error) {
	t.Helper()
	posts := 0
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			page := secondaryFixture(t, "search-number-142-2026-ND-CP.html")
			if detail == "uncertain" {
				page = bytes.ReplaceAll(page, []byte("142/2026/NĐ-CP"), []byte("1367"))
			}
			if detail == "unstable" {
				posts++
				page = bytes.ReplaceAll(page, []byte("docid=218029"), []byte(fmt.Sprintf("docid=%d", 218029+posts)))
			}
			if strings.HasPrefix(key, "chinhphu:named:367/") {
				page = bytes.ReplaceAll(page, []byte("142/2026/NĐ-CP"), []byte("1367/QĐ-TTg"))
			}
			_, _ = w.Write(page)
			return
		}
		if r.URL.Query().Get("docid") != "" {
			name := detail
			if detail == "unstable" || detail == "uncertain" || detail == "unknown-issuer" || detail == "wrong-unknown" {
				name = "detail-218029.html"
			}
			body := secondaryFixture(t, name)
			if detail == "unknown-issuer" || detail == "wrong-unknown" {
				body = bytes.ReplaceAll(body, []byte("Chính phủ"), []byte("Quốc hội"))
			}
			if detail == "wrong-unknown" {
				body = bytes.ReplaceAll(body, []byte("142/2026/NĐ-CP"), []byte("143/2026/NĐ-CP"))
			}
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write(secondaryFixture(t, "home.html"))
	}))
	t.Cleanup(web.Close)
	probeCalls := 0
	vb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if vbplResponse == "unstable-probe" {
			probeCalls++
			data := response(fmt.Sprintf(`[{"id":"id%d","docNum":"142/2026/NĐ-CP","issueDate":"2026-04-30","agencyName":"Chính phủ"}]`, probeCalls), 1)
			_, _ = fmt.Fprint(w, data)
			return
		}
		_, _ = fmt.Fprint(w, vbplResponse)
	}))
	t.Cleanup(vb.Close)
	cp := chinhphu.New(chinhphu.WithBaseURLs(web.URL, web.URL), chinhphu.WithLimits(chinhphu.Limits{RequestsPerSecond: 100000, Burst: 1, InFlight: 1}), chinhphu.WithBackoffs())
	s := New(testClient(vb.URL), WithChinhphu(cp))
	run := stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":1},"chinhphu":{"issuer_policy_version":1,"central_issuer_forms":["Chính phủ"],"keywords":["law"],"named_numbers":["142/2026/NĐ-CP"]}}`)}}
	if strings.HasPrefix(key, "chinhphu:named:367/") {
		run.Snapshot.SourceList = bytes.ReplaceAll(run.Snapshot.SourceList, []byte("142/2026/NĐ-CP"), []byte("367/QĐ-TTg@2026-02-28"))
	}
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	out, err := s.Process(context.Background(), env, run, stage.Item{Key: key})
	if err != nil {
		return stage.Resolution{}, nil, err
	}
	if out.Artifact == nil {
		return stage.Resolution{}, nil, fmt.Errorf("no artifact")
	}
	var report map[string]any
	b, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(out.Artifact.Path), "secondary.json"))
	if err != nil {
		return stage.Resolution{}, nil, err
	}
	if err = json.Unmarshal(b, &report); err != nil {
		return stage.Resolution{}, nil, err
	}
	resolved, err := s.Resolve(context.Background(), env, run, stage.Item{Key: key}, *out.Artifact)
	return resolved, report, err
}
func TestAdmit_ZeroVBPLMatchAdmits(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	candidates, ok := report["candidates"].([]any)
	if !ok || len(candidates) != 1 || candidates[0].(map[string]any)["state"] != "admit_secondary" {
		t.Fatal("unexpected candidate count or state")
	}
}

func candidateState(t *testing.T, report map[string]any) string {
	t.Helper()
	candidates, ok := report["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatal("unexpected candidate count or state")
	}
	return candidates[0].(map[string]any)["state"].(string)
}
func TestAdmit_DifferentDateNeitherSeedsNorBlocks(t *testing.T) {
	_, report, err := secondaryRun(t, response(`[{"id":"other","docNum":"142/2026/NĐ-CP","issueDate":"2026-04-29","agencyName":"Chính phủ"}]`, 1), "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	if got := candidateState(t, report); got != "admit_secondary" {
		t.Fatal(got)
	}
}
func TestAdmit_UniqueVBPLMatchSeedsPrimary(t *testing.T) {
	_, report, err := secondaryRun(t, response(`[{"id":"same","docNum":"142/2026/NĐ-CP","issueDate":"2026-04-30","agencyName":"Chính phủ"}]`, 1), "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	if got := candidateState(t, report); got != "seed_primary" {
		t.Fatal(got)
	}
}
func TestAdmit_LookupFailureDefers(t *testing.T) {
	res, report, err := secondaryRun(t, `<html>gateway error</html>`, "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		var retry *stage.RetryableError
		if !errors.As(err, &retry) || retry.GapKind != "source_unavailable" {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if len(res.Gaps) != 1 || res.Gaps[0].Kind != "source_unavailable" || candidateState(t, report) != "incomplete" {
		t.Fatal("failed lookup did not retain only its source gap")
	}
}
func TestAdmit_CollisionIsIdentityAmbiguous(t *testing.T) {
	res, report, err := secondaryRun(t, response(`[{"id":"same","docNum":"142/2026/NĐ-CP","issueDate":"2026-04-30","agencyName":"Quốc hội"}]`, 1), "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	if got := candidateState(t, report); got != "ambiguous" || len(res.Gaps) == 0 || res.Gaps[0].Kind != "source_identity_ambiguous" {
		t.Fatalf("state=%s gaps=%+v", got, res.Gaps)
	}
}
func TestAdmit_MultipleMatchesRemainAmbiguous(t *testing.T) {
	reply := response(`[{"id":"one","docNum":"142/2026/NĐ-CP","issueDate":"2026-04-30","agencyName":"Chính phủ"},{"id":"two","docNum":"142/2026/NĐ-CP","issueDate":"2026-04-30","agencyName":"Chính phủ"}]`, 2)
	res, report, err := secondaryRun(t, reply, "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	if candidateState(t, report) != "ambiguous" || len(res.Gaps) == 0 || res.Gaps[0].Kind != "source_identity_ambiguous" {
		t.Fatal("multiple identities were not ambiguous")
	}
}

func TestAdmit_ProvincialOutOfScope(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "detail-215941.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	if got := candidateState(t, report); got != "provincial" {
		t.Fatal(got)
	}
}

func TestNamed_SubstringHitRejected(t *testing.T) {
	res, report, err := secondaryRun(t, response("[]", 0), "detail-218029.html", "chinhphu:named:367/QĐ-TTg@2026-02-28")
	if err != nil {
		t.Fatal(err)
	}
	candidates := report["candidates"].([]any)
	if len(candidates) != 0 || len(res.Gaps) != 1 || res.Gaps[0].Kind != "named_document_missing" {
		t.Fatal("substring candidate was not rejected")
	}
	if rejected, ok := report["filtered"].([]any); !ok || len(rejected) != 1 {
		t.Fatal("missing named filtering evidence")
	}
	if outcomes, ok := report["filter_outcomes"].([]any); !ok || len(outcomes) != 2 {
		t.Fatal("missing per-pass filtering outcomes")
	}
}

func TestSecondary_WireIdentityBindsIssuerFilter(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	passes := report["passes"].([]any)
	evidence := passes[0].(map[string]any)["evidence"].(map[string]any)
	query := evidence["query"].(map[string]any)
	if query["issuer_filter"] != "none" {
		t.Fatalf("query=%v", query)
	}
}

func TestNamed_UnstableSearchCannotAdmit(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "unstable", "chinhphu:named:142/2026/NĐ-CP")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range report["candidates"].([]any) {
		state := v.(map[string]any)["state"]
		if state == "admit_secondary" || state == "seed_primary" {
			t.Fatal("unstable named search admitted a candidate")
		}
	}
}

func TestIssuer_RecordsPolicyAndMatchedForm(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	c := report["candidates"].([]any)[0].(map[string]any)
	if report["issuer_policy_version"] != float64(1) || c["matched_form"] != "Chính phủ" {
		t.Fatal("missing policy version or matched issuer form")
	}
}

func TestNamed_UncertainListingNumberReachesDetail(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "uncertain", "chinhphu:named:142/2026/NĐ-CP")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(report["candidates"].([]any)); got != 1 {
		t.Fatalf("candidate count %d", got)
	}
}

func TestProbe_UnstablePassesKeepUnionEvidence(t *testing.T) {
	_, report, err := secondaryRun(t, "unstable-probe", "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	c := report["candidates"].([]any)[0].(map[string]any)
	comparisons, ok := c["comparisons"].([]any)
	if c["state"] != "incomplete" || !ok || len(comparisons) != 3 {
		t.Fatal("unstable probe did not retain union comparisons")
	}
}

func TestIssuer_UnknownPreservesIdentityEvidence(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "unknown-issuer", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	c := report["candidates"].([]any)[0].(map[string]any)
	if c["state"] != "invalid" || c["issuer_form"] != "Quốc hội" || c["number"] != "142/2026/NĐ-CP" {
		t.Fatal("unknown issuer identity evidence was lost")
	}
}

func TestSecondary_CandidateEvidenceReferencesSearchRecord(t *testing.T) {
	report := secondaryReport{QueryIdentity: "item", WireIdentity: "wire", SourceListHash: "source", IssuerPolicyVersion: 1}
	candidate := secondaryCandidate{ID: "1"}
	evidence := secondaryCandidateEvidence(report, candidate)
	if evidence.SearchPath != "secondary.json" || evidence.ItemIdentity != "item" || evidence.Candidate.ID != "1" {
		t.Fatal("candidate evidence is not bound to search record")
	}
}

func TestNamed_UnequalDetailNumberIsIneligible(t *testing.T) {
	res, report, err := secondaryRun(t, response("[]", 0), "wrong-unknown", "chinhphu:named:142/2026/NĐ-CP")
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, gap := range res.Gaps {
		if gap.Kind == "named_document_missing" {
			missing = true
		}
	}
	if candidateState(t, report) != "ineligible" || !missing {
		t.Fatal("known unequal number was treated as unresolved")
	}
}

func TestSecondary_SourceFailuresAreClassified(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{chinhphu.ErrNotFound, "not_found"},
		{chinhphu.ErrTooLarge, "response_too_large"},
		{&chinhphu.Failure{Origin: "source_data", Cause: "invalid_payload", Endpoint: "detail"}, "invalid_source_data"},
	} {
		if got := secondaryFailure("chinhphu", tc.err); got.Kind != tc.want {
			t.Errorf("kind=%s want=%s", got.Kind, tc.want)
		}
	}
}

func TestSecondary_ArtifactHasVersion(t *testing.T) {
	_, report, err := secondaryRun(t, response("[]", 0), "detail-218029.html", "chinhphu:keyword:law")
	if err != nil {
		t.Fatal(err)
	}
	if report["version"] != float64(1) {
		t.Fatal("missing artifact version")
	}
}
