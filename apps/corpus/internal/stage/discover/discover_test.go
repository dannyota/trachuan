package discover

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dannyota/traquydinh/apps/corpus/internal/artifact"
	"github.com/dannyota/traquydinh/apps/corpus/internal/stage"
	"github.com/dannyota/traquydinh/apps/corpus/internal/vbpl"
)

type redirectTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = r.target.Scheme, r.target.Host
	clone.URL = &u
	return r.base.RoundTrip(clone)
}

type testPage struct {
	ids   []string
	total int
}

func exercise(t *testing.T, pages []testPage, wantPasses int, stable bool, wantIDs []string) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if calls >= len(pages) {
			t.Errorf("unexpected request %d", calls)
			http.Error(w, "unexpected", 500)
			return
		}
		var body struct {
			Page      int    `json:"pageNumber"`
			Size      int    `json:"pageSize"`
			Sort      string `json:"sortBy"`
			Direction string `json:"sortDirection"`
			Group     bool   `json:"groupVbpl"`
			Option    string `json:"optionDoc"`
			Mode      string `json:"matchMode"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Size != 50 || body.Sort != "issueDate" || body.Direction != "desc" || body.Group || body.Option != "title" || body.Mode != "all_words" {
			t.Errorf("request: %+v", body)
		}
		p := pages[calls]
		calls++
		items := make([]map[string]string, 0, len(p.ids))
		for _, id := range p.ids {
			items = append(items, map[string]string{"id": id, "title": fmt.Sprintf("%s-%d", id, calls)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": p.total, "items": items}})
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	http.DefaultTransport = redirectTransport{target, old}
	defer func() { http.DefaultTransport = old }()
	s := New(vbpl.New("test"))
	env := stage.Env{Artifacts: artifact.Store{Root: t.TempDir()}}
	run := stage.Run{ID: 1, Snapshot: stage.Snapshot{SourceList: json.RawMessage(`{"country":"vn","vbpl":{"sweep_agencies":["62","908"]}}`)}}
	items, err := s.Plan(context.Background(), env, run)
	if err != nil || len(items) != 1 || items[0].Key != "vbpl:sweep" || items[0].InputHash != "" {
		t.Fatalf("plan=%+v err=%v", items, err)
	}
	out, err := s.Process(context.Background(), env, run, items[0])
	if err != nil {
		t.Fatal(err)
	}
	if out.Artifact == nil || out.State != "done" {
		t.Fatalf("output=%+v", out)
	}
	data, err := os.ReadFile(filepath.Join(out.Artifact.Path, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		QueryKey string                     `json:"query_key"`
		Passes   int                        `json:"passes"`
		Totals   []int                      `json:"totals"`
		Stable   bool                       `json:"stable"`
		Items    map[string]json.RawMessage `json:"items"`
	}
	if decodeErr := json.Unmarshal(data, &report); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	sum := sha256.Sum256([]byte(`{"agencies":["62","908"],"keyword":""}`))
	wantQuery := fmt.Sprintf("vbpl:sweep@%x", sum[:8])
	if report.Passes != wantPasses || report.Stable != stable || len(report.Totals) != wantPasses || report.QueryKey != wantQuery {
		t.Fatalf("report=%+v", report)
	}
	for _, id := range wantIDs {
		if _, ok := report.Items[id]; !ok {
			t.Errorf("missing id %s", id)
		}
	}
	lastTitles := make(map[string]string)
	for i, p := range pages {
		for _, id := range p.ids {
			lastTitles[id] = fmt.Sprintf("%s-%d", id, i+1)
		}
	}
	for id, raw := range report.Items {
		var item struct {
			Title string `json:"title"`
		}
		if itemErr := json.Unmarshal(raw, &item); itemErr != nil || item.Title != lastTitles[id] {
			t.Errorf("latest item %s: %s, err=%v", id, raw, itemErr)
		}
	}
	if len(report.Items) != len(wantIDs) {
		t.Errorf("items=%v", report.Items)
	}
	data, err = os.ReadFile(filepath.Join(out.Artifact.Path, "passes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var passes [][]json.RawMessage
	if passErr := json.Unmarshal(data, &passes); passErr != nil || len(passes) != wantPasses {
		t.Fatalf("passes=%s err=%v", data, passErr)
	}
	rows, err := s.Rows(context.Background(), *out.Artifact)
	if err != nil || rows != nil {
		t.Fatalf("rows=%v err=%v", rows != nil, err)
	}
	resolved, err := s.Resolve(context.Background(), env, run, items[0], *out.Artifact)
	if err != nil || resolved.RunRows == nil {
		t.Fatalf("resolution=%+v err=%v", resolved, err)
	}
	var gaps []stage.Gap
	if !stable {
		gaps = []stage.Gap{{Kind: "listing_unstable", Detail: fmt.Sprint(wantPasses)}}
	}
	if !reflect.DeepEqual(resolved.Gaps, gaps) {
		t.Errorf("gaps=%+v want=%+v", resolved.Gaps, gaps)
	}
	if calls != len(pages) {
		t.Errorf("calls=%d want=%d", calls, len(pages))
	}
}
func TestDiscover_StableAfterTwoPasses(t *testing.T) {
	exercise(t, []testPage{{[]string{"a", "b"}, 2}, {[]string{"b", "a"}, 2}}, 2, true, []string{"a", "b"})
}
func TestDiscover_EqualTotalReplacementTriggersThirdPass(t *testing.T) {
	exercise(t, []testPage{{[]string{"a"}, 1}, {[]string{"b"}, 1}, {[]string{"b"}, 1}}, 3, true, []string{"a", "b"})
}
func TestDiscover_UnstableAfterThreePassesIsGap(t *testing.T) {
	exercise(t, []testPage{{[]string{"a"}, 1}, {[]string{"b"}, 1}, {[]string{"c"}, 1}}, 3, false, []string{"a", "b", "c"})
}
func TestDiscover_DedupesShiftedPages(t *testing.T) {
	ids := make([]string, 50)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	shifted := append([]string(nil), ids...)
	shifted[0] = "new"
	expected := append(append([]string(nil), ids...), "new")
	exercise(t, []testPage{{ids, 51}, {[]string{"0"}, 51}, {shifted, 51}, {[]string{"0"}, 51}, {shifted, 51}, {[]string{"0"}, 51}}, 3, true, expected)
}
