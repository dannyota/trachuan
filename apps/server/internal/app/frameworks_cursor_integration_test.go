//go:build integration

package app

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/server/internal/mcpapi"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/stretchr/testify/require"
)

type frameworkLawResolver struct{ engine *retrieval.Engine }

func (r frameworkLawResolver) Resolve(context.Context, string, string) (mcpapi.Lease, error) {
	return mcpapi.Lease{Engine: r.engine, Release: r.engine.Release().Rel, Done: func() {}}, nil
}

func TestFrameworkMCP_CursorsCannotCrossCorpora(t *testing.T) {
	frameworks, _, _ := frameworkFixture(t, false, "20990101")
	law := frameworkLawEngine(t)
	session, _ := frameworkTransport(t, frameworks, &frameworkTransportAuth{}, frameworkLawResolver{law})
	lawArgs := map[string]any{"country": "vn", "query": "ngân hàng", "as_of": "2099-01-02", "limit": 1}
	frameworkArgs := map[string]any{"corpus": "frameworks", "query": "quartz", "limit": 1}
	lawPage, _, failed := frameworkWireCall(t, session, "search", lawArgs)
	require.False(t, failed)
	lawCursor, ok := lawPage["next_cursor"].(string)
	require.True(t, ok)
	require.NotEmpty(t, lawCursor)
	frameworkPage, _, failed := frameworkWireCall(t, session, "search", frameworkArgs)
	require.False(t, failed)
	frameworkCursor, ok := frameworkPage["next_cursor"].(string)
	require.True(t, ok)
	require.NotEmpty(t, frameworkCursor)
	for _, tc := range []struct {
		args   map[string]any
		cursor string
	}{
		{lawArgs, lawCursor}, {frameworkArgs, frameworkCursor},
	} {
		tc.args["cursor"] = tc.cursor
		_, _, failed = frameworkWireCall(t, session, "search", tc.args)
		require.False(t, failed)
	}
	for _, tc := range []struct {
		args   map[string]any
		cursor string
	}{
		{lawArgs, frameworkCursor}, {frameworkArgs, lawCursor},
	} {
		tc.args["cursor"] = tc.cursor
		value, _, rejected := frameworkWireCall(t, session, "search", tc.args)
		require.True(t, rejected)
		require.Equal(t, "invalid_request", value["error"].(map[string]any)["code"])
	}
	lawRead, _, failed := frameworkWireCall(t, session, "read", map[string]any{"country": "vn", "items": []any{map[string]any{"handle": "vn:invented:1#dieu-1"}}})
	require.False(t, failed)
	lawNext := lawRead["items"].([]any)[0].(map[string]any)["next"].(string)
	require.NotEmpty(t, lawNext)
	frameworkRead, _, failed := frameworkWireCall(t, session, "read", map[string]any{"corpus": "frameworks", "items": []any{map[string]any{"handle": "fw:fw-alpha:1#A-1"}}})
	require.False(t, failed)
	frameworkNext := frameworkRead["items"].([]any)[0].(map[string]any)["next"].(string)
	require.NotEmpty(t, frameworkNext)
	for _, tc := range []struct {
		selector map[string]any
		next     string
	}{
		{map[string]any{"country": "vn"}, frameworkNext}, {map[string]any{"corpus": "frameworks"}, lawNext},
	} {
		tc.selector["items"] = []any{map[string]any{"next": tc.next}}
		value, _, rejected := frameworkWireCall(t, session, "read", tc.selector)
		require.False(t, rejected)
		require.Equal(t, "invalid_next", value["items"].([]any)[0].(map[string]any)["error"])
	}
}
