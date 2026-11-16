package serve

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestReadText_ParagraphsNotesAndTables(t *testing.T) {
	units := textUnits("Publisher", "first\n\nsecond\n \n", []string{"note"}, []string{"table"})
	var kinds, texts []string
	for _, unit := range units {
		kinds = append(kinds, unit.Kind)
		texts = append(texts, unit.Text)
	}
	require.Equal(t, []string{"title_original", "body", "body", "body", "note", "table"}, kinds)
	require.Equal(t, []string{"Publisher", "first", "second", " ", "note", "table"}, texts)
}

func TestReadParts_PreserveTextAndBoundSerializedUnits(t *testing.T) {
	for _, text := range []string{strings.Repeat("invented word ", 100), strings.Repeat("a\u0301", 200), strings.Repeat("\"\\\n", 100)} {
		parts, ok := readUnitParts(ReadUnit{Kind: "body", Text: text}, 150)
		require.True(t, ok)
		require.Greater(t, len(parts), 1)
		var joined string
		for i, part := range parts {
			require.LessOrEqual(t, serializedSize(part), 150)
			require.Equal(t, i+1, part.Part)
			require.Equal(t, len(parts), part.Parts)
			require.True(t, utf8.ValidString(part.Text))
			joined += part.Text
		}
		require.Equal(t, text, joined)
	}
	_, ok := readUnitParts(ReadUnit{Kind: "outline", Outline: &Outline{Title: strings.Repeat("x", 200)}}, 100)
	require.False(t, ok)
	parts, ok := readUnitParts(ReadUnit{Kind: "mapping", Edge: &MappingEdge{Evidence: strings.Repeat("invented ", 100)}}, 200)
	require.True(t, ok)
	var evidence string
	for _, part := range parts {
		evidence += part.Edge.Evidence
	}
	require.Equal(t, strings.Repeat("invented ", 100), evidence)
}

func TestReadPage_ContinuationsAndCallCap(t *testing.T) {
	e := testEngine()
	p := e.project(fakeCaller())
	c := continuation{Operation: "read", Release: e.release, Access: p.version, Handle: "fw:fw-beta:1#A-1", Mode: "control"}
	c.Digest = digestValue([]string{c.Handle, c.Mode})
	units := []ReadUnit{{Kind: "body", Text: strings.Repeat("invented ", 100)}}
	page, cost := e.readPage(units, c, p, 250, 150)
	require.Empty(t, page.Error)
	require.NotEmpty(t, page.Next)
	require.False(t, page.Complete)
	require.LessOrEqual(t, cost, 250)
	deferred, cost := e.readPage(units, c, p, 0, 150)
	require.Equal(t, "call_cap", deferred.NotRead)
	require.NotEmpty(t, deferred.Next)
	require.Zero(t, cost)
	var text string
	for {
		for _, part := range page.Units {
			text += part.Text
		}
		if page.Next == "" {
			break
		}
		var err error
		c, err = e.decodeContinuation(page.Next, "read", c.Digest, p)
		require.NoError(t, err)
		page, _ = e.readPage(units, c, p, 250, 150)
	}
	require.True(t, page.Complete)
	require.Equal(t, units[0].Text, text)
	bad, _ := e.readPage(units, continuation{Unit: 99}, p, 250, 150)
	require.Equal(t, "invalid_next", bad.Error)
}
