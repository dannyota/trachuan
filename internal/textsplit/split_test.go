package textsplit

import (
	"reflect"
	"testing"
)

func TestRough_Count(t *testing.T) {
	for _, tc := range []struct {
		runes int
		want  int
	}{{0, 0}, {1, 1}, {2, 1}, {3, 1}, {4, 2}, {1024, 512}, {1025, 512}, {1026, 513}} {
		if got := Rough(tc.runes); got != tc.want {
			t.Errorf("Rough(%d) = %d, want %d", tc.runes, got, tc.want)
		}
	}
}

func TestSplit_Events(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		lead  int
		limit int
		want  []Event
	}{
		{name: "empty", lead: 8, limit: 4},
		{name: "whitespace only", body: " \t\n\u2003", lead: 8, limit: 4},
		{
			name: "fits whole preserves internal whitespace", body: " \tα\n β \u2003", limit: 2,
			want: []Event{{Ranges: []Range{{2, 6}}}},
		},
		{
			name: "odd rune fits boundary", body: "abcde", limit: 2,
			want: []Event{{Ranges: []Range{{0, 5}}}},
		},
		{
			name: "oversized word stays intact", body: "abcdef", limit: 2,
			want: []Event{{Ranges: []Range{{0, 6}}, Separator: ' '}},
		},
		{
			name: "trim and pack lines", body: " a \n bb\n\n ccc \n dddd ", limit: 4,
			want: []Event{
				{Ranges: []Range{{1, 2}, {5, 7}, {10, 13}}, Separator: '\n'},
				{Ranges: []Range{{16, 20}}, Separator: '\n'},
			},
		},
		{
			name: "pack unicode separated words", body: " aa\tbb\u2003ccc dd ", limit: 4,
			want: []Event{
				{Ranges: []Range{{1, 3}, {4, 6}, {7, 10}}, Separator: ' '},
				{Ranges: []Range{{11, 13}}, Separator: ' '},
			},
		},
		{
			name: "flush lines before words", body: "a\nbb cc dd ee\nf", limit: 3,
			want: []Event{
				{Ranges: []Range{{0, 1}}, Separator: '\n'},
				{Ranges: []Range{{2, 4}, {5, 7}}, Separator: ' '},
				{Ranges: []Range{{8, 10}, {11, 13}}, Separator: ' '},
				{Ranges: []Range{{14, 15}}, Separator: '\n'},
			},
		},
		{
			name: "whole includes lead newline in limit", body: "abcd", lead: 2, limit: 3,
			want: []Event{{Ranges: []Range{{0, 4}}, AttachLead: true}},
		},
		{
			name: "repeat lead on packed lines", body: "aa\nbb\ncc", lead: 2, limit: 3,
			want: []Event{
				{Ranges: []Range{{0, 2}}, Separator: '\n', AttachLead: true},
				{Ranges: []Range{{3, 5}}, Separator: '\n', AttachLead: true},
				{Ranges: []Range{{6, 8}}, Separator: '\n', AttachLead: true},
			},
		},
		{
			name: "repeat lead on packed words", body: "aa bb cc dd", lead: 2, limit: 4,
			want: []Event{
				{Ranges: []Range{{0, 2}, {3, 5}}, Separator: ' ', AttachLead: true},
				{Ranges: []Range{{6, 8}, {9, 11}}, Separator: ' ', AttachLead: true},
			},
		},
		{
			name: "release after attached piece before pending words", body: "aa bb cccccc d\ne", lead: 4, limit: 4,
			want: []Event{
				{Ranges: []Range{{0, 2}}, Separator: ' ', AttachLead: true},
				{ReleaseLead: true},
				{Ranges: []Range{{3, 5}, {6, 12}}, Separator: ' '},
				{Ranges: []Range{{13, 14}}, Separator: ' '},
				{Ranges: []Range{{15, 16}}, Separator: '\n'},
			},
		},
		{
			name: "release precedes pending word emission", body: "a cccccc d", lead: 4, limit: 4,
			want: []Event{
				{ReleaseLead: true},
				{Ranges: []Range{{0, 1}, {2, 8}}, Separator: ' '},
				{Ranges: []Range{{9, 10}}, Separator: ' '},
			},
		},
		{
			name: "release first word includes joining newline", body: "x", lead: 2, limit: 1,
			want: []Event{
				{ReleaseLead: true},
				{Ranges: []Range{{0, 1}}, Separator: ' '},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []rune(tc.body)
			if got := Split(body, tc.lead, tc.limit); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Split() = %#v, want %#v", got, tc.want)
			}
			if string(body) != tc.body {
				t.Fatal("Split changed the input")
			}
		})
	}
}
