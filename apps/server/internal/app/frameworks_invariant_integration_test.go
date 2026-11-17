//go:build integration

package app

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestFrameworkMCP_TwinDatabasesPreserveReducedResponses(t *testing.T) {
	left, _, _ := frameworkFixture(t, false, "20990101")
	right, _, _ := frameworkFixture(t, true, "20990101")
	for _, scopes := range [][]string{nil, {"fw-gamma"}} {
		a, logsA := frameworkTransport(t, left, &frameworkTransportAuth{scopes: scopes})
		b, logsB := frameworkTransport(t, right, &frameworkTransportAuth{scopes: scopes})
		for _, op := range []struct {
			name  string
			args  map[string]any
			paged bool
		}{
			{"corpus_status", map[string]any{}, false},
			{"search", map[string]any{"query": "quartz", "limit": 1}, true},
			{"search", map[string]any{"query": "FW-ALPHA A-1"}, false},
			{"search", map[string]any{"query": "quartz", "frameworks": []string{"fw-alpha"}, "versions": []string{"fw:fw-alpha:1"}}, false},
			{"read", map[string]any{"items": []any{map[string]any{"handle": "fw:fw-alpha:1#A-1"}}}, true},
			{"read", map[string]any{"items": []any{map[string]any{"handle": "fw:fw-alpha:1"}}}, false},
			{"mappings", map[string]any{"handle": "fw:fw-alpha:1#A-1", "limit": 50}, true},
			{"mappings", map[string]any{"from": "fw:fw-alpha:1", "to": "fw:fw-beta:1", "relation": []string{"related"}}, false},
			{"quality_gaps", map[string]any{}, true},
			{"quality_gaps", map[string]any{"versions": []string{"fw:fw-alpha:1"}, "kind": "text_incomplete"}, false},
		} {
			op.args["corpus"] = "frameworks"
			for range 2 {
				pages := frameworkTwinWalk(t, a, b, op.name, op.args)
				if op.paged {
					require.Greater(t, pages, 1, op.name)
				}
			}
		}
		for _, op := range []struct {
			name string
			args map[string]any
			code string
		}{
			{"search", map[string]any{"query": "quartz", "frameworks": []string{"fw-unknown"}}, "invalid_request"},
			{"search", map[string]any{"query": "quartz", "cursor": "invented-bad-cursor"}, "invalid_request"},
			{"mappings", map[string]any{"handle": "bad"}, "invalid_handle"},
			{"quality_gaps", map[string]any{"versions": []string{"fw:fw-unknown:1"}}, "invalid_request"},
			{"read", map[string]any{"items": []any{map[string]any{"handle": "bad"}, map[string]any{"next": "bad"}, map[string]any{"handle": "fw:fw-alpha:1#ABSENT"}}}, ""},
			{"read", map[string]any{"items": []any{map[string]any{"handle": "fw:fw-alpha:1#A-1"}, map[string]any{"handle": "vn:test#dieu-1"}}}, "invalid_request"},
		} {
			op.args["corpus"] = "frameworks"
			value, first, failed := frameworkWireCall(t, a, op.name, op.args)
			_, second, _ := frameworkWireCall(t, b, op.name, op.args)
			require.Equal(t, first, second, op.name)
			require.Equal(t, op.code != "", failed, op.name)
			if op.code != "" {
				require.Equal(t, op.code, value["error"].(map[string]any)["code"])
			} else {
				items := value["items"].([]any)
				for i, code := range []string{"invalid_handle", "invalid_next", "not_found"} {
					require.Equal(t, code, items[i].(map[string]any)["error"])
				}
			}
		}
		require.NotContains(t, logsA.String(), "quartz")
		require.NotContains(t, logsB.String(), "nebula")
	}
	a, _ := frameworkTransport(t, left, &frameworkTransportAuth{scopes: []string{"*"}})
	b, _ := frameworkTransport(t, right, &frameworkTransportAuth{scopes: []string{"*"}})
	for _, name := range []string{"read", "search", "mappings"} {
		args := map[string]any{"corpus": "frameworks"}
		switch name {
		case "read":
			args["items"] = []any{map[string]any{"handle": "fw:fw-alpha:1#A-1"}}
		case "search":
			args["query"] = "FW-ALPHA A-1"
		case "mappings":
			args["handle"] = "fw:fw-alpha:1#A-1"
		}
		first, rawA, failedA := frameworkWireCall(t, a, name, args)
		second, rawB, failedB := frameworkWireCall(t, b, name, args)
		require.False(t, failedA)
		require.False(t, failedB)
		require.Equal(t, first["access_set_version"], second["access_set_version"])
		require.NotEqual(t, rawA, rawB, name)
	}
}

func frameworkTwinWalk(t *testing.T, a, b *mcp.ClientSession, name string, original map[string]any) int {
	t.Helper()
	args := make(map[string]any, len(original))
	for key, value := range original {
		args[key] = value
	}
	for page := 1; page <= 20; page++ {
		first, rawA, failedA := frameworkWireCall(t, a, name, args)
		_, rawB, failedB := frameworkWireCall(t, b, name, args)
		require.False(t, failedA, name)
		require.False(t, failedB, name)
		require.Equal(t, rawA, rawB, name)
		require.NotEmpty(t, first["access_set_version"], name)
		if name == "corpus_status" {
			versions := first["versions"].(map[string]any)
			licensed := versions["fw:fw-alpha:1"].(map[string]any)
			require.Equal(t, false, licensed["readable"])
			require.Nil(t, licensed["sources"])
			require.NotEmpty(t, versions["fw:fw-beta:1"].(map[string]any)["sources"])
			require.Equal(t, float64(12), first["totals"].(map[string]any)["controls"])
		}
		if name == "read" {
			items := first["items"].([]any)
			item := items[0].(map[string]any)
			next, _ := item["next"].(string)
			if next == "" {
				require.Equal(t, true, item["complete"])
				return page
			}
			require.Equal(t, false, item["complete"])
			args["items"] = []any{map[string]any{"next": next}}
		} else {
			next, _ := first["next_cursor"].(string)
			if next == "" {
				return page
			}
			args["cursor"] = next
		}
	}
	t.Fatal("frameworks continuation did not terminate")
	return 0
}
