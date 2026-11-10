package discover

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestDiscover_CentralListingStablePasses(t *testing.T) {
	run := stage.Run{Snapshot: stage.Snapshot{SourceList: json.RawMessage(`{"country":"vn","vbpl":{"selection_version":1,"named_numbers":["71/2025/QH15"]}}`)}}
	items, err := New(nil).Plan(context.Background(), stage.Env{}, run)
	if err != nil {
		t.Fatal(err)
	}
	want := []stage.Item{{Key: "vbpl:central"}, {Key: "vbpl:named:71/2025/QH15"}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items=%+v want=%+v", items, want)
	}
	var recorded struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err = json.Unmarshal([]byte(scopeFixture(t, "list-central-size5-page1.json")), &recorded); err != nil {
		t.Fatal(err)
	}
	itemsJSON, err := json.Marshal(recorded.Data.Items)
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic total bounds the recorded items to one test page.
	p := response(string(itemsJSON), len(recorded.Data.Items))
	resolved, _, dir, err := scopeRun(t, "", []string{p, p})
	if err != nil || len(resolved.Gaps) != 0 {
		t.Fatalf("resolution=%+v err=%v", resolved, err)
	}
	var evidence struct {
		Stable bool              `json:"stable"`
		Passes []json.RawMessage `json:"passes"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &evidence); err != nil || !evidence.Stable || len(evidence.Passes) != 2 {
		t.Fatalf("report=%s err=%v", data, err)
	}

}

// Synthetic responses use the gateway's listing envelope.
func scopeRun(t *testing.T, entry string, responses []string) (stage.Resolution, stage.Output, string, error) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var query struct {
			PageSize int      `json:"pageSize"`
			Level    string   `json:"agencyLevel"`
			Agencies []string `json:"agencyIds"`
			Keyword  string   `json:"keyword"`
			Option   string   `json:"optionDoc"`
			Mode     string   `json:"matchMode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			t.Error(err)
		}
		if query.Level != "TRUNG_UONG" || len(query.Agencies) != 0 {
			t.Errorf("invalid listing filter: %+v", query)
		}
		if entry == "" && (query.PageSize != 1000 || query.Keyword != "" || query.Option != "title" || query.Mode != "all_words") {
			t.Errorf("invalid central query: %+v", query)
		}
		if entry != "" && (query.Option != "docNum" || query.Mode != "exact") {
			t.Errorf("invalid named query: %+v", query)
		}
		if calls >= len(responses) {
			t.Errorf("unexpected request %d", calls)
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte(responses[calls]))
		calls++
	}))
	t.Cleanup(server.Close)
	snapshot := `{"country":"vn","vbpl":{"selection_version":1,"sweep_agencies":["62"],"keywords":["ngân hàng"],"named_numbers":["` + entry + `"]}}`
	if entry == "" {
		snapshot = `{"country":"vn","vbpl":{"selection_version":1,"sweep_agencies":["62"],"keywords":["ngân hàng"]}}`
	}
	run := stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceList: json.RawMessage(snapshot)}}
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	s := New(testClient(server.URL))
	key := "vbpl:central"
	if entry != "" {
		key = "vbpl:named:" + entry
	}
	out, err := s.Process(context.Background(), env, run, stage.Item{Key: key})
	if err != nil {
		return stage.Resolution{}, out, "", err
	}
	if out.Artifact == nil {
		t.Fatal("missing artifact")
	}
	path := env.Artifacts.Abs(out.Artifact.Path)
	res, err := s.Resolve(context.Background(), env, run, stage.Item{Key: key}, *out.Artifact)
	return res, out, path, err
}

func response(items string, total int) string {
	return fmt.Sprintf(`{"success":true,"data":{"total":%d,"items":%s}}`, total, items)
}
func scopeFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "vbpl", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestDiscover_LegacySnapshotUnchanged(t *testing.T) {
	run := stage.Run{Snapshot: stage.Snapshot{SourceList: json.RawMessage(`{"country":"vn","vbpl":{"sweep_agencies":["62"]}}`)}}
	items, err := New(nil).Plan(context.Background(), stage.Env{}, run)
	if err != nil || len(items) != 1 || items[0].Key != "vbpl:sweep" || New(nil).Recipe(run.Snapshot) != stage.RecipeHash("discover", "vbpl-discover/1") {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}
func TestDiscover_UnstableListingIsGap(t *testing.T) {
	a, b, c := response(`[{"id":"a"}]`, 1), response(`[{"id":"b"}]`, 1), response(`[{"id":"c"}]`, 1)
	res, _, _, err := scopeRun(t, "", []string{a, b, c})
	if err != nil || len(res.Gaps) != 1 || res.Gaps[0].Kind != "listing_unstable" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
func TestDiscover_PartialListingProvesNothing(t *testing.T) {
	p := response(`[]`, 1)
	res, _, _, err := scopeRun(t, "", []string{p, p, p})
	if err != nil || len(res.Gaps) != 1 || res.Gaps[0].Kind != "listing_unstable" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
func TestDiscover_LocalSelectionSupports(t *testing.T) {
	p := response(`[{"id":"a","title":"Ngân hàng số","agencyIds":["62"]},{"id":"b","title":"Other"}]`, 2)
	_, _, dir, err := scopeRun(t, "", []string{p, p})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "selected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Items []struct {
			ID    string            `json:"id"`
			Rules []json.RawMessage `json:"rules"`
		} `json:"items"`
	}
	if err = json.Unmarshal(data, &value); err != nil || len(value.Items) != 1 || value.Items[0].ID != "a" || len(value.Items[0].Rules) != 2 {
		t.Fatalf("selected=%s err=%v", data, err)
	}
}
func TestDiscover_PagesStoredWithRawHashes(t *testing.T) {
	p := response(`[{"id":"a"}]`, 1)
	_, out, dir, err := scopeRun(t, "", []string{p, p})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Sources, []string{"vbpl"}) {
		t.Fatal(out.Sources)
	}
	f, err := os.Open(filepath.Join(dir, "pages", "p1-1.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(gz)
	if err != nil || string(raw) != p {
		t.Fatalf("raw=%s err=%v", raw, err)
	}
	report, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if !bytes.Contains(report, []byte(fmt.Sprintf("%x", sum))) {
		t.Fatalf("missing hash: %s", report)
	}
}
func TestNamed_UniqueJoinsSeeds(t *testing.T) {
	p := scopeFixture(t, "lookup-71-2025-QH15.json")
	res, _, _, err := scopeRun(t, "71/2025/QH15", []string{p, p})
	if err != nil || len(res.Gaps) != 0 || res.RunRows == nil {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
func TestNamed_DifferentDateNeitherSeedsNorBlocks(t *testing.T) {
	p := scopeFixture(t, "lookup-367-QD-TTg.json")
	res, _, _, err := scopeRun(t, "367/QĐ-TTg@2026-02-28", []string{p, p})
	if err != nil || len(res.Gaps) != 1 || res.Gaps[0].Kind != "named_document_missing" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
func TestNamed_ZeroAndManyAreGaps(t *testing.T) {
	for _, tc := range []struct{ entry, file, want string }{{"142/2026/NĐ-CP", "lookup-142-2026-ND-CP.json", "named_document_missing"}, {"367/QĐ-TTg@2013-01-01", "lookup-367-QD-TTg.json", "named_document_missing"}} {
		t.Run(tc.entry, func(t *testing.T) {
			p := scopeFixture(t, tc.file)
			res, _, _, err := scopeRun(t, tc.entry, []string{p, p})
			if err != nil || len(res.Gaps) != 1 || res.Gaps[0].Kind != tc.want {
				t.Fatalf("res=%+v err=%v", res, err)
			}
		})
	}
}
func TestNamed_ManyEqualCandidatesAreAmbiguous(t *testing.T) {
	p := response(`[{"id":"a","docNum":"71/2025/QH15","issueDate":"2025-01-01"},{"id":"b","docNum":"71/2025/QH15","issueDate":"2025-01-01"}]`, 2)
	res, _, _, err := scopeRun(t, "71/2025/QH15", []string{p, p})
	if err != nil || len(res.Gaps) != 1 || res.Gaps[0].Kind != "named_document_ambiguous" || !strings.Contains(res.Gaps[0].Detail, "a,b") {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
func TestNamed_UnparsableDateIsAmbiguous(t *testing.T) {
	p := response(`[{"id":"a","docNum":"71/2025/QH15","issueDate":"bad"}]`, 1)
	res, _, _, err := scopeRun(t, "71/2025/QH15", []string{p, p})
	if err != nil || len(res.Gaps) != 1 || res.Gaps[0].Kind != "named_document_ambiguous" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}
func TestNamed_LookupFailureIsGapNotAbsence(t *testing.T) {
	_, _, _, err := scopeRun(t, "71/2025/QH15", []string{`<html>error</html>`, `<html>error</html>`, `<html>error</html>`, `<html>error</html>`})
	var retry *stage.RetryableError
	if !errors.As(err, &retry) || retry.GapKind != "source_unavailable" {
		t.Fatalf("err=%v", err)
	}
}

func TestPlan_SecondaryQueriesAreDistinctAndSorted(t *testing.T) {
	run := stage.Run{Snapshot: stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":1},"chinhphu":{"issuer_policy_version":1,"central_issuer_forms":["Chính phủ"],"keywords":["b","a","a"],"named_numbers":["71/2025/QH15","71/2025/QH15"]}}`)}}
	items, err := New(nil).Plan(context.Background(), stage.Env{}, run)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	want := []string{"chinhphu:keyword:a", "chinhphu:keyword:b", "chinhphu:named:71/2025/QH15", "vbpl:central"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func TestRecipe_SecondaryConfigurationBumpsRecipe(t *testing.T) {
	base := stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":1}}`)}
	secondary := stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":1},"chinhphu":{"issuer_policy_version":1,"central_issuer_forms":["Chính phủ"],"keywords":["law"]}}`)}
	legacyRecipe := stage.RecipeHash("discover", "vbpl-discover/selection-2", secondary.SourceListHash)
	if New(nil).Recipe(secondary) == legacyRecipe || New(nil).Recipe(base) != stage.RecipeHash("discover", "vbpl-discover/selection-2", base.SourceListHash) {
		t.Fatal("recipe does not bind secondary implementation")
	}
	for _, snapshot := range []stage.Snapshot{base, secondary} {
		snapshot.SourceListHash = "first"
		first := New(nil).Recipe(snapshot)
		snapshot.SourceListHash = "second"
		if first == New(nil).Recipe(snapshot) {
			t.Fatal("recipe does not bind the source list hash")
		}
	}
}

func TestPlan_InvalidSecondaryPolicyFails(t *testing.T) {
	run := stage.Run{Snapshot: stage.Snapshot{SourceList: json.RawMessage(`{"vbpl":{"selection_version":1},"chinhphu":{"keywords":["law"]}}`)}}
	if _, err := New(nil).Plan(context.Background(), stage.Env{}, run); err == nil {
		t.Fatal("missing issuer policy accepted")
	}
}

func TestDiscover_UnstableListingSelectsEarlierObservation(t *testing.T) {
	a := response(`[{"id":"a","title":"Ngân hàng số"}]`, 1)
	b := response(`[{"id":"b","title":"Other"}]`, 1)
	c := response(`[{"id":"c","title":"Other"}]`, 1)
	res, _, dir, err := scopeRun(t, "", []string{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	var selected selectedFile
	data, err := os.ReadFile(filepath.Join(dir, "selected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &selected); err != nil {
		t.Fatal(err)
	}
	if len(res.Gaps) != 1 || res.Gaps[0].Kind != "listing_unstable" || len(selected.Items) != 1 || selected.Items[0].ID != "a" || selected.Items[0].Page.Pass != 1 {
		t.Fatalf("gaps=%+v selected=%+v", res.Gaps, selected.Items)
	}
}

func TestDiscover_UnstableListingRetainsEachObservedRule(t *testing.T) {
	a := response(`[{"id":"a","title":"Ngân hàng"}]`, 1)
	b := response(`[{"id":"a","title":"Other","agencyIds":["62"]},{"id":"b"}]`, 2)
	c := response(`[{"id":"c"}]`, 1)
	_, _, dir, err := scopeRun(t, "", []string{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	var selected selectedFile
	data, err := os.ReadFile(filepath.Join(dir, "selected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &selected); err != nil {
		t.Fatal(err)
	}
	if len(selected.Items) != 1 || len(selected.Items[0].Rules) != 2 {
		t.Fatalf("selected=%+v", selected.Items)
	}
	pages := map[string]int{}
	for _, rp := range selected.Items[0].RulePages {
		pages[rp.Rule.Kind] = rp.Page.Pass
	}
	if pages["term"] != 1 || pages["agency"] != 2 {
		t.Fatalf("rule pages=%v", pages)
	}
}
