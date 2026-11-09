package lookup

import (
	"encoding/json"
	"testing"
)

func TestLookupResult_EmptyIsAbsenceOnlyWhenComplete(t *testing.T) {
	zero := 0
	b := Builder{TotalReported: &zero, EndedNormally: true}
	if b.Result().ProvesAbsence() {
		t.Fatal("missing response proved absence")
	}
	b.Pages = []Page{{Total: &zero}}
	if b.Result().ProvesAbsence() {
		t.Fatal("unrecorded response proved absence")
	}
	b.Pages[0].SHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	b.Pages[0].Path = "responses/1.json"
	r := b.Result()
	if !r.ProvesAbsence() {
		t.Fatal("recorded complete empty response did not prove absence")
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got Result
	if decodeErr := json.Unmarshal(data, &got); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	again, err := json.Marshal(got)
	if err != nil || string(data) != string(again) {
		t.Fatalf("round trip: %s %s %v", data, again, err)
	}
}

func TestLookupResult_CompletenessRules(t *testing.T) {
	one, two := 1, 2
	tests := []struct {
		name string
		b    Builder
		want Completeness
	}{
		{"disagreeing totals", Builder{TotalReported: &one, EndedNormally: true, Pages: []Page{{Total: &two, ResultIDs: []string{"a"}}}}, Incomplete},
		{"duplicate only page", Builder{TotalReported: &one, EndedNormally: true, Pages: []Page{{Total: &one, ResultIDs: []string{"a"}}, {Total: &one, ResultIDs: []string{"a"}}}}, Incomplete},
		{"unknown total", Builder{EndedNormally: true, Pages: []Page{{ResultIDs: []string{"a"}}}}, Incomplete},
		{"failed page", Builder{TotalReported: &one, EndedNormally: true, Failed: true, Pages: []Page{{Total: &one, ResultIDs: []string{"a"}}}}, Incomplete},
		{"parser uncertainty", Builder{TotalReported: &one, EndedNormally: true, Uncertain: true, Pages: []Page{{Total: &one, ResultIDs: []string{"a"}}}}, Incomplete},
		{"exact match", Builder{TotalReported: &two, EndedNormally: true, Pages: []Page{{Total: &two, ResultIDs: []string{"a"}}, {Total: &two, ResultIDs: []string{"b"}}}}, Complete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.b.Result().Evidence.Completeness; got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestLookupResult_UnknownVersionFails(t *testing.T) {
	var r Result
	if err := json.Unmarshal([]byte(`{"version":2,"items":[],"evidence":{}}`), &r); err == nil {
		t.Fatal("unknown version accepted")
	}
}
