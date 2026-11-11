package fetch

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestControl_Sample(t *testing.T) {
	const key = "vn:vbpl:66518"
	first := controlSample(key, "20991231_1")
	if controlSample(key, "20991231_1") != first {
		t.Fatal("sample changed")
	}
	changed := false
	for i := 2; i < 100; i++ {
		if controlSample(key, fmt.Sprintf("20991231_%d", i)) != first {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("release does not affect sample")
	}
	count := 0
	for i := 0; i < 10000; i++ {
		if controlSample(fmt.Sprintf("vn:vbpl:%d", i), "20991231_1") {
			count++
		}
	}
	if count < 400 || count > 600 {
		t.Fatalf("sample count %d", count)
	}
}

func TestControl_Comparison(t *testing.T) {
	for _, tc := range []struct{ name, tree, files, report, want string }{
		{"same", `{"key":1,"nodes":[{"id":1,"text":"a"}]}`, `[]`, `{}`, "same"},
		{"ids", `{"key":2,"nodes":[{"id":2,"text":"a"}]}`, `[]`, `{}`, "tree_ids_only"},
		{"text", `{"key":2,"nodes":[{"id":2,"text":"b"}]}`, `[]`, `{}`, "changed"},
		{"files", `{"key":1,"nodes":[{"id":1,"text":"a"}]}`, `[1]`, `{}`, "changed"},
		{"invalid", `not json`, `[]`, `{}`, "changed"},
		{"large number", `{"key":2,"nodes":[{"id":2,"text":"a"}],"number":9007199254740993}`, `[]`, `{}`, "changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &baseline{tree: []byte(`{"key":1,"nodes":[{"id":1,"text":"a"}]}`), filesJSON: []byte(`[]`), reportJSON: []byte(`{}`)}
			got, _ := compareControl(base, []byte(tc.tree), []byte(tc.files), []byte(tc.report))
			if got != tc.want {
				t.Fatalf("outcome %q want %q", got, tc.want)
			}
		})
	}
}

func TestControl_FullRead(t *testing.T) {
	e := entry("a", "a")
	e["size"] = 3
	e["updatedAt"] = "2026-04-14 14:41:19"
	f := &fixtureServer{files: []map[string]any{e}, bodies: map[string]string{"a": "one"}}
	s, env := setup(t, f)
	now := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	first := process(t, s, env)
	base := baselineFromOutput(t, env, first)
	s.WithCache(cacheFixture(t, "one"))
	s.loadBaseline = func(context.Context, stage.Env, stage.Run, stage.Item) (*baseline, bool, error) {
		return base, false, nil
	}
	rel := ""
	for i := 1; i < 1000; i++ {
		r := fmt.Sprintf("20991231_%d", i)
		if controlSample("vn:vbpl:66518", r) {
			rel = r
			break
		}
	}
	if rel == "" {
		t.Fatal("no sampled release")
	}
	f.downloads.Store(0)
	out, err := s.Process(context.Background(), env, stage.Run{Country: "vn", Rel: rel}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil || f.downloads.Load() != 1 || out.Message != "read=control outcome=same" {
		t.Fatalf("downloads=%d message=%q err=%v", f.downloads.Load(), out.Message, err)
	}
	f.detailBody = `{"success":true,"data":{"id":"66518","updatedDate":null}}`
	out, err = s.Process(context.Background(), env, stage.Run{Country: "vn", Rel: rel}, stage.Item{Key: "vn:vbpl:66518"})
	if err != nil || out.Message != "read=control" {
		t.Fatalf("message=%q err=%v", out.Message, err)
	}
}

func TestControl_Message(t *testing.T) {
	for _, decision := range []readDecision{{reason: "reused"}, {reason: "null_date"}, {reason: "control", outcome: "same"}} {
		message := decision.message()
		parsed := parseReadMessage(message)
		if parsed.reason != decision.reason || parsed.outcome != decision.outcome || !strings.HasPrefix(message, "read=") {
			t.Fatalf("round trip %q: %+v", message, parsed)
		}
	}
}

func TestControl_LargeNumericChange(t *testing.T) {
	b := &baseline{tree: []byte(`{"id":1,"number":9007199254740992}`), filesJSON: []byte(`[]`), reportJSON: []byte(`{}`)}
	got, _ := compareControl(b, []byte(`{"id":2,"number":9007199254740993}`), []byte(`[]`), []byte(`{}`))
	if got != "changed" {
		t.Fatalf("outcome = %q, want changed", got)
	}
}
