package expand

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
)

func TestExpand_QualifyingKindsOutwardOnly(t *testing.T) {
	codes := map[string]source.RelationCode{"A": {Kind: "cites"}, "B": {Kind: "other"}}
	detail := []byte(`{"data":{"references":[{"referenceType":"A","targetDocument":{"id":42}},{"referenceType":"B","targetDocument":{"id":"3"}},{"referenceType":"X","targetDocument":{"id":"4"}}]}}`)
	diagram := []byte(`{"data":{"documentNamesByType":{"A":[{"id":"42"},{"id":"uuid-a"}]}}}`)
	got, err := Qualify("vn:vbpl:1", detail, diagram, codes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 3 || got.Targets[0].TargetKey != "vn:vbpl:42" || got.Targets[1].TargetKey != "vn:vbpl:42" || got.Targets[2].TargetKey != "vn:vbpl:uuid-a" || len(got.UnknownCodes) != 1 || got.UnknownCodes[0] != "X" {
		t.Fatalf("observation = %#v", got)
	}
}

func TestExpand_ExcludesRenditionsAndSelfLinks(t *testing.T) {
	codes := map[string]source.RelationCode{"A": {Kind: "references"}}
	detail := []byte(`{"data":{"references":[{"referenceType":"A","targetDocument":{"id":"1"}},{"referenceType":"A","targetDocument":{"id":"VBPQTA_2"}},{"referenceType":"A","targetDocument":{"id":"bad id"}},{"referenceType":"A","targetDocument":{"docNum":"123/2025/QH15"}}]}}`)
	got, err := Qualify("vn:vbpl:1", detail, []byte(`{"data":{}}`), codes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 0 || len(got.NumberOnly) != 1 || got.NumberOnly[0].Number != "123/2025/QH15" {
		t.Fatalf("observation = %#v", got)
	}
}

func TestExpand_UsesSeedCountryAndDeduplicates(t *testing.T) {
	codes := map[string]source.RelationCode{"A": {Kind: "cites"}}
	detail := []byte(`{"data":{"references":[{"referenceType":"A","targetDocument":{"id":"2"}},{"referenceType":"A","targetDocument":{"id":"2"}},{"referenceType":"A","targetDocument":{"docNum":"  law  "}},{"referenceType":"A","targetDocument":{"docNum":"law"}}]}}`)
	diagram := []byte(`{"data":{"documentNamesByType":{"A":[{"id":"2"},{"id":"2"},{"name":"  diagram number  "},{"name":"diagram number"}]}}}`)
	got, err := Qualify("kh:vbpl:1", detail, diagram, codes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 2 || got.Targets[0].TargetKey != "kh:vbpl:2" || got.Targets[1].TargetKey != "kh:vbpl:2" {
		t.Fatalf("targets = %#v", got.Targets)
	}
	if len(got.NumberOnly) != 2 || got.NumberOnly[0].Number != "diagram number" || got.NumberOnly[1].Number != "law" {
		t.Fatalf("number only = %#v", got.NumberOnly)
	}
}

func TestExpand_AllQualifyingKinds(t *testing.T) {
	kinds := []string{"legal_basis", "implements", "guided_by", "amends_supplements", "replaces", "repeals", "suspends", "consolidates", "cites", "references"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			codes := map[string]source.RelationCode{"A": {Kind: kind}}
			detail := []byte(`{"data":{"references":[{"referenceType":"A","targetDocument":{"id":"2"}}],"inboundReferences":[{"referenceType":"A","targetDocument":{"id":"3"}}]}}`)
			obs, err := Qualify("vn:vbpl:1", detail, []byte(`{"data":{}}`), codes)
			if err != nil || len(obs.Targets) != 1 || obs.Targets[0].TargetKey != "vn:vbpl:2" || obs.Targets[0].Kind != kind {
				t.Fatalf("observation=%#v err=%v", obs, err)
			}
		})
	}
}

func TestExpand_NumberOnlyWithUnknownCodeIsAudited(t *testing.T) {
	detail := []byte(`{"data":{"references":[{"referenceType":"unknown","targetDocument":{"docNum":"LAW-1"}}]}}`)
	obs, err := Qualify("vn:vbpl:1", detail, []byte(`{"data":{}}`), nil)
	if err != nil || len(obs.UnknownCodes) != 1 || len(obs.NumberOnly) != 1 || obs.NumberOnly[0].Number != "LAW-1" {
		t.Fatalf("observation=%#v err=%v", obs, err)
	}
}
