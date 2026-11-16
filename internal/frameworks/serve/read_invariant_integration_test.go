//go:build integration

package serve

import (
	"context"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/retrieval"
	"github.com/stretchr/testify/require"
)

func assertReadInvariant(t *testing.T, pair operationTwin) {
	t.Helper()
	request := ReadRequest{Items: []ReadItem{
		{Handle: "fw:fw-alpha:2#A-1"}, {Handle: "fw:fw-beta:1#A-1"},
		{Handle: "fw:fw-gamma:1#A-1"}, {Handle: "fw:fw-alpha:2"},
		{Handle: "fw:fw-alpha:2#A"}, {Handle: "fw:fw-alpha:2#A-999"},
		{Handle: "bad"}, {Next: "bad"},
	}}
	chains := map[string][]ReadUnit{}
	pages := map[string]int{}
	for page := 0; ; page++ {
		require.Less(t, page, 30)
		pair.reset()
		a, ea := pair.a.Read(context.Background(), request, pair.caller)
		b, eb := pair.b.Read(context.Background(), request, pair.caller)
		pair.compare(t, a, b, ea, eb)
		require.NoError(t, ea)
		require.Len(t, a.Items, len(request.Items))
		used := 0
		var next []ReadItem
		for _, item := range a.Items {
			for _, unit := range item.Units {
				cost := serializedSize(unit)
				require.LessOrEqual(t, cost, retrieval.PartSize)
				used += cost
			}
			if item.Error != "" {
				require.Contains(t, []string{"not_found", "invalid_handle", "invalid_next"}, item.Error)
				require.Empty(t, item.Next)
				continue
			}
			chains[item.Handle] = append(chains[item.Handle], item.Units...)
			pages[item.Handle]++
			if item.Next != "" {
				require.False(t, item.Complete)
				next = append(next, ReadItem{Next: item.Next})
			} else {
				require.True(t, item.Complete)
			}
			if item.NotRead != "" {
				require.Equal(t, "call_cap", item.NotRead)
				require.Empty(t, item.Units)
				require.NotEmpty(t, item.Next)
			}
		}
		require.LessOrEqual(t, used, retrieval.ReadCallCap)
		if len(next) == 0 {
			break
		}
		request.Items = next
	}
	units := chains["fw:fw-alpha:2#A-1"]
	require.NotEmpty(t, units)
	require.Equal(t, "head", units[0].Kind)
	require.True(t, units[0].Head.TextWithheld)
	require.Greater(t, pages["fw:fw-alpha:2#A-1"], 1)
	mappings, heads := 0, 0
	for _, unit := range units {
		require.Contains(t, []string{"head", "mapping", "outline"}, unit.Kind)
		if unit.Kind == "mapping" {
			mappings++
			if unit.Edge.Source != "invented-public" {
				require.Empty(t, unit.Edge.Evidence)
				require.Nil(t, unit.Edge.EvidenceTruncated)
			}
		}
		if unit.Kind == "head" {
			heads++
		}
	}
	require.Equal(t, 206, mappings)
	require.Equal(t, 1, heads)
	family := chains["fw:fw-alpha:2#A"]
	require.Len(t, family, 4)
	for i, citation := range []string{"A-1", "A-2", "A-3"} {
		require.Equal(t, "outline", family[i+1].Kind)
		require.Equal(t, citation, family[i+1].Outline.Citation)
	}
	version := chains["fw:fw-alpha:2"]
	require.Len(t, version, 2)
	require.Equal(t, "A", version[1].Outline.Citation)
}

func assertEntitledRead(t *testing.T, e *Engine, caller Caller, term string, repeats int) {
	t.Helper()
	request := ReadRequest{Items: []ReadItem{{Handle: "fw:fw-alpha:2#A-1"}}}
	note, table, evidence := "", "", ""
	parts := 0
	order := map[string]int{"head": 0, "title_original": 1, "body": 2, "note": 3, "table": 4, "mapping": 5, "outline": 6}
	previous := -1
	for page := 0; ; page++ {
		require.Less(t, page, 30)
		response, err := e.Read(context.Background(), request, caller)
		require.NoError(t, err)
		require.Len(t, response.Items, 1)
		item := response.Items[0]
		require.Empty(t, item.Error)
		for _, unit := range item.Units {
			require.GreaterOrEqual(t, order[unit.Kind], previous)
			previous = order[unit.Kind]
			if unit.Part > 0 {
				parts++
			}
			switch unit.Kind {
			case "head":
				require.False(t, unit.Head.TextWithheld)
			case "note":
				note += unit.Text
			case "table":
				table += unit.Text
			case "mapping":
				require.Nil(t, unit.Edge.EvidenceTruncated)
				if strings.HasPrefix(unit.Edge.Source, "invented-001-") {
					evidence += unit.Edge.Evidence
				}
			}
		}
		if item.Next == "" {
			require.True(t, item.Complete)
			require.Greater(t, page, 0)
			break
		}
		require.False(t, item.Complete)
		request.Items = []ReadItem{{Next: item.Next}}
	}
	require.Greater(t, parts, 2)
	require.Equal(t, strings.Repeat("Invented "+term+" note. ", repeats), note)
	require.Equal(t, "Invented "+term+" table row.", table)
	require.Equal(t, strings.Repeat("Invented "+term+" evidence. ", repeats), evidence)
}
